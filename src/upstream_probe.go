package main

// One bounded, anonymous startup probe separates upstream network failures from
// local session failures. It sends no saved cookies, passwords or account data.
import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"
)

type upstreamProbeResult struct {
	Stage, Failure string
	Code           int
}

func probeUpstream(ctx context.Context, target string, client *http.Client) upstreamProbeResult {
	var stage atomic.Value
	stage.Store("dns")
	trace := &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { stage.Store("dns") },
		ConnectStart:      func(string, string) { stage.Store("connect") },
		TLSHandshakeStart: func() { stage.Store("tls") },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { stage.Store("tls-completed") },
		WroteRequest:      func(httptrace.WroteRequestInfo) { stage.Store("response-headers") },
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, target, nil)
	if err != nil {
		return upstreamProbeResult{Stage: "request", Failure: "invalid-request"}
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		failure := "network"
		var network net.Error
		if errors.As(err, &network) && network.Timeout() {
			failure = "timeout"
		}
		if errors.Is(err, context.Canceled) {
			failure = "cancelled"
		}
		return upstreamProbeResult{Stage: stage.Load().(string), Failure: failure}
	}
	resp.Body.Close()
	return upstreamProbeResult{Stage: "http", Code: resp.StatusCode}
}

func logUpstreamReachability(ctx context.Context) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	for _, target := range []struct{ label, url string }{{"portal", portal}, {"oauth", oauth}} {
		result := probeUpstream(ctx, target.url, client)
		log.Printf("Upstream reachability: endpoint=%s stage=%s status=%d failure=%s", target.label, result.Stage, result.Code, result.Failure)
	}
}
