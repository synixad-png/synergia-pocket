package main

// Bounded startup diagnostics for fixed public hosts. No credentials, HTTP
// payloads, proxying, TLS exceptions or user-controlled destinations are used.
import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"sync"
	"time"
)

func logNetworkPath(parent context.Context) {
	var hosts sync.WaitGroup
	for _, host := range []string{"synergia.librus.pl", "api.librus.pl", "portal.librus.pl", "render.com"} {
		hosts.Add(1)
		go func(host string) {
			defer hosts.Done()
			ctx, cancel := context.WithTimeout(parent, 18*time.Second)
			defer cancel()
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				log.Printf("Network path: host=%s dns_error=%v", host, err)
				return
			}
			log.Printf("Network path: host=%s dns=%v", host, addresses)
			var checks sync.WaitGroup
			count4, count6 := 0, 0
			for _, addr := range addresses {
				family := "ipv4"
				if addr.IP.To4() == nil {
					family = "ipv6"
					if count6 >= 8 {
						continue
					}
					count6++
				} else {
					if count4 >= 8 {
						continue
					}
					count4++
				}
				checks.Add(1)
				go func(ip, family string) { defer checks.Done(); probeAddress(ctx, host, ip, family) }(addr.IP.String(), family)
			}
			checks.Wait()
		}(host)
	}
	hosts.Wait()
}

func probeAddress(parent context.Context, host, ip, family string) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
	if err != nil {
		log.Printf("Network path: host=%s ip=%s family=%s tcp_error=%v", host, ip, family, err)
		return
	}
	defer conn.Close()
	log.Printf("Network path: host=%s ip=%s family=%s tcp=ok", host, ip, family)
	secure := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	tlsCtx, tlsCancel := context.WithTimeout(parent, 4*time.Second)
	defer tlsCancel()
	if err := secure.HandshakeContext(tlsCtx); err != nil {
		log.Printf("Network path: host=%s ip=%s family=%s tls_error=%v", host, ip, family, err)
		return
	}
	log.Printf("Network path: host=%s ip=%s family=%s tls=ok", host, ip, family)
}
