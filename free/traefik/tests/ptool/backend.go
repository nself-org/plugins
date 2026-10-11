package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

var hits atomic.Int64

// backend serves every route target from one process: http and https ports from
// the route model, plus 9999 /hits (how many requests reached any upstream).
// Answers carry no addresses, so a body is equal whichever proxy relayed it.
func backend(r routes) {
	_, ports := upstreams(r)
	cert, key := selfSigned("backend", []string{"backend"}, 1)
	pair, err := tls.X509KeyPair(cert, key)
	must(err)
	errc := make(chan error, len(ports)+1)
	for p, scheme := range ports {
		srv := &http.Server{Addr: ":" + strconv.Itoa(p), Handler: http.HandlerFunc(serve)}
		if scheme == "https" {
			srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{pair}}
			go func() { errc <- srv.ListenAndServeTLS("", "") }()
		} else {
			go func() { errc <- srv.ListenAndServe() }()
		}
	}
	go func() {
		errc <- http.ListenAndServe(":9999", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, hits.Load())
		}))
	}()
	die(<-errc)
}

type echo struct {
	Port        string `json:"port"`
	Method      string `json:"method"`
	URI         string `json:"uri"`
	Host        string `json:"host"`
	Proto       string `json:"x_forwarded_proto"`
	ForwardedBy bool   `json:"x_forwarded_for_present"`
	RealIP      bool   `json:"x_real_ip_present"`
	Tenant      string `json:"x_notes_tenant"`
	BodyLen     int64  `json:"body_len"`
	BodySHA     string `json:"body_sha256"`
}

func serve(w http.ResponseWriter, r *http.Request) {
	hits.Add(1)
	port := "0"
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		_, port, _ = net.SplitHostPort(a.String())
	}
	w.Header().Set("X-Backend", port)
	p := r.URL.Path
	switch {
	case strings.EqualFold(r.Header.Get("Upgrade"), "websocket"):
		websocket(w, r)
	case strings.Contains(p, "/big."):
		ctype := map[string]string{"txt": "text/plain", "json": "application/json", "html": "text/html", "png": "image/png", "css": "text/css"}[p[strings.LastIndex(p, "big.")+4:]]
		w.Header().Set("Content-Type", ctype)
		_, _ = io.WriteString(w, strings.Repeat("parity-payload 0123456789 abcdefghij\n", 400))
	default:
		h := sha256.New()
		n, _ := io.Copy(h, r.Body)
		e := echo{Port: port, Method: r.Method, URI: r.URL.RequestURI(), Host: r.Host, Proto: r.Header.Get("X-Forwarded-Proto"),
			ForwardedBy: r.Header.Get("X-Forwarded-For") != "", RealIP: r.Header.Get("X-Real-Ip") != "",
			Tenant: r.Header.Get("X-Notes-Tenant"), BodyLen: n, BodySHA: hex.EncodeToString(h.Sum(nil))}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e)
	}
}

func websocket(w http.ResponseWriter, r *http.Request) {
	sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no hijack", 500)
		return
	}
	c, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer c.Close()
	fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		base64.StdEncoding.EncodeToString(sum[:]))
	_ = rw.Flush()
	_, _ = io.Copy(c, rw)
}
