package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnonymousProbeDoesNotSendAccountCookiesOrFollowRedirect(t *testing.T) {
	redirected := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("probe sent credentials")
		}
		if r.URL.Path == "/private" {
			redirected = true
		}
		w.Header().Set("Location", "/private")
		w.WriteHeader(302)
	}))
	defer s.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result := probeUpstream(context.Background(), s.URL, client)
	if result.Code != 302 || result.Failure != "" || redirected {
		t.Fatal(result, redirected)
	}
}
