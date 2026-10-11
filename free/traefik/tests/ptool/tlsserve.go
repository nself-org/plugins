package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// tlsServe proves that the proxy serves every https route's own certificate: for each server name of each route with
// TLS it opens a TLS connection with that SNI and requires the leaf to list the name among its SANs (a proxy that
// cannot read the private key falls back to its built-in default certificate, which has no such SAN), then sends one
// HTTPS request and requires an HTTP answer of any status (the routing matrix is proven elsewhere; this is about
// the key being readable). P7-DEPL-27 (D-0508): the keys of that case are 0600 and owned by a non-root uid.
func tlsServe(r routes, target string) {
	var names []string
	seen := map[string]bool{}
	for _, rt := range r.Routes {
		if rt.TLS == nil {
			continue
		}
		for _, n := range rt.ServerNames {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		die(fmt.Errorf("tlsserve: the fixture has no https route"))
	}
	bad := 0
	for _, n := range names {
		sans, err := servedSANs(target+":443", n)
		if err != nil {
			fmt.Printf("FAIL %s: TLS handshake: %v\n", n, err)
			bad++
			continue
		}
		if !contains(sans, n) {
			fmt.Printf("FAIL %s: served certificate has SANs %v (the default certificate means the key was unreadable)\n", n, sans)
			bad++
			continue
		}
		status, err := httpsStatus(target, n)
		if err != nil {
			fmt.Printf("FAIL %s: https request: %v\n", n, err)
			bad++
			continue
		}
		fmt.Printf("ok   %s: SAN present, https answered %d\n", n, status)
	}
	if bad > 0 {
		die(fmt.Errorf("tlsserve: %d of %d https names failed", bad, len(names)))
	}
	fmt.Printf("tlsserve: %d https names served with their own certificate\n", len(names))
}

func servedSANs(addr, sni string) ([]string, error) {
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.ConnectionState().PeerCertificates[0].DNSNames, nil
}

func httpsStatus(target, host string) (int, error) {
	cl := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, target+":443")
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest("GET", "https://"+host+"/", nil)
	if err != nil {
		return 0, err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
