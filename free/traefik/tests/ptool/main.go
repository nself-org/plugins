// Command ptool is the parity suite's helper binary (tests only, never shipped):
//
//	ptool plan    --routes F                 upstream aliases and ports the backend must serve
//	ptool backend --routes F                 deterministic upstream for every route target
//	ptool gencert --routes F --out D [--gen N]   self-signed lineages like P7-LIVE-15 writes them
//	ptool rawdata --url U                    every Traefik router/service/middleware must be enabled
//	ptool serial  --addr A --sni H           leaf serial of the certificate served for H
//	ptool matrix  --routes F --nginx H --traefik H [--tls]   request-level comparison
//
// stdlib only; exit 0 pass, 1 mismatch or failure.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type routes struct {
	DefaultServer struct {
		HTTP struct {
			RedirectHTTPS bool `json:"redirect_https"`
		} `json:"http"`
	} `json:"default_server"`
	Defaults struct {
		MaxBodyBytes int64 `json:"max_body_bytes"`
	} `json:"defaults"`
	Zones []struct {
		Name string  `json:"name"`
		Rate *string `json:"rate"`
	} `json:"zones"`
	Routes []route `json:"routes"`
}

type route struct {
	ID          string   `json:"id"`
	ServerNames []string `json:"server_names"`
	Listen      struct {
		HTTP  bool `json:"http"`
		HTTPS bool `json:"https"`
	} `json:"listen"`
	TLS *struct {
		SSLDir string `json:"ssl_dir"`
	} `json:"tls"`
	RateLimit *struct {
		Zone  string `json:"zone"`
		Burst int    `json:"burst"`
	} `json:"rate_limit"`
	BlockedPaths []struct {
		Status int `json:"status"`
	} `json:"blocked_paths"`
	Locations []struct {
		Path      string `json:"path"`
		Match     string `json:"match"`
		WebSocket bool   `json:"websocket"`
		Upstream  *struct {
			Scheme string `json:"scheme"`
			Host   string `json:"host"`
			Port   int    `json:"port"`
		} `json:"upstream"`
		RateLimit *struct {
			Zone  string `json:"zone"`
			Burst int    `json:"burst"`
		} `json:"rate_limit"`
		StaticRoot *string `json:"static_root"`
	} `json:"locations"`
}

func loadRoutes(path string) routes {
	b, err := os.ReadFile(path)
	if err != nil {
		die(err)
	}
	var r routes
	if err := json.Unmarshal(b, &r); err != nil {
		die(err)
	}
	return r
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "ptool:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		die(fmt.Errorf("usage: ptool plan|backend|gencert|rawdata|serial|matrix"))
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	rf := fs.String("routes", "/routes.json", "routes.json")
	out := fs.String("out", "/ssl", "ssl dir (gencert)")
	gen := fs.Int("gen", 1, "generation number (gencert)")
	url := fs.String("url", "", "rawdata URL")
	addr := fs.String("addr", "", "host:port (serial)")
	sni := fs.String("sni", "", "server name (serial)")
	ngx := fs.String("nginx", "nginx", "nginx container")
	trf := fs.String("traefik", "traefik", "traefik container")
	req := fs.String("require", "", "router that must exist (rawdata)")
	_ = fs.Parse(args)
	switch cmd {
	case "plan":
		plan(loadRoutes(*rf))
	case "backend":
		backend(loadRoutes(*rf))
	case "gencert":
		gencert(loadRoutes(*rf), *out, *gen)
	case "rawdata":
		rawdata(*url, *req)
	case "serial":
		s, err := serialErr(*addr, *sni)
		must(err)
		fmt.Println(s)
	case "matrix":
		matrix(loadRoutes(*rf), *ngx, *trf)
	default:
		die(fmt.Errorf("unknown command %q", cmd))
	}
}

// upstreams returns the distinct hosts and the port->scheme map of every route target.
func upstreams(r routes) ([]string, map[int]string) {
	hosts, ports := map[string]bool{}, map[int]string{}
	for _, rt := range r.Routes {
		for _, l := range rt.Locations {
			if l.Upstream != nil {
				hosts[l.Upstream.Host] = true
				ports[l.Upstream.Port] = l.Upstream.Scheme
			}
		}
	}
	var hs []string
	for h := range hosts {
		hs = append(hs, h)
	}
	sort.Strings(hs)
	return hs, ports
}

func plan(r routes) {
	hs, ports := upstreams(r)
	for _, h := range hs {
		fmt.Println("alias", h)
	}
	for p, s := range ports {
		fmt.Println("port", p, s)
	}
}

func gencert(r routes, out string, gen int) {
	names := map[string]map[string]bool{}
	for _, rt := range r.Routes {
		if rt.TLS == nil {
			continue
		}
		if names[rt.TLS.SSLDir] == nil {
			names[rt.TLS.SSLDir] = map[string]bool{}
		}
		for _, n := range rt.ServerNames {
			names[rt.TLS.SSLDir][n] = true
		}
	}
	base := filepath.Join(out, "certificates")
	for dir, set := range names {
		var dns []string
		for n := range set {
			dns = append(dns, n)
		}
		sort.Strings(dns)
		genDir := fmt.Sprintf(".%s.gen-%d", dir, gen)
		cert, key := selfSigned(dir, dns, int64(gen))
		must(os.MkdirAll(filepath.Join(base, genDir), 0o755))
		must(os.WriteFile(filepath.Join(base, genDir, "fullchain.pem"), cert, 0o644))
		must(os.WriteFile(filepath.Join(base, genDir, "privkey.pem"), key, 0o644))
		link := filepath.Join(base, dir)
		_ = os.Remove(link)
		must(os.Symlink(genDir, link))
	}
	tok := filepath.Join(out, ".acme-webroot", ".well-known", "acme-challenge")
	must(os.MkdirAll(tok, 0o755))
	must(os.WriteFile(filepath.Join(tok, "parity-token"), []byte("token-content\n"), 0o644))
}

func must(err error) {
	if err != nil {
		die(err)
	}
}

func selfSigned(cn string, dns []string, serial int64) (certPEM, keyPEM []byte) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(serial + 1000), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour), DNSNames: dns,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	must(err)
	kb, err := x509.MarshalPKCS8PrivateKey(k)
	must(err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
}

func serialErr(addr, sni string) (string, error) {
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.ConnectionState().PeerCertificates[0].SerialNumber.String(), nil
}

func rawdata(url, require string) {
	resp, err := http.Get(url)
	if err != nil {
		die(err)
	}
	defer resp.Body.Close()
	var raw map[string]map[string]map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		die(err)
	}
	bad, total := 0, 0
	for _, kind := range []string{"routers", "services", "middlewares"} {
		for name, item := range raw[kind] {
			total++
			if item["status"] != "enabled" || item["error"] != nil {
				bad++
				fmt.Printf("NOT ENABLED %s %s: %v %v\n", kind, name, item["status"], item["error"])
			}
		}
	}
	if _, ok := raw["routers"][require]; require != "" && !ok {
		die(fmt.Errorf("router %s not loaded yet", require))
	}
	if len(raw["routers"]) == 0 || bad > 0 {
		die(fmt.Errorf("%d of %d items not enabled (routers: %d)", bad, total, len(raw["routers"])))
	}
	fmt.Printf("rawdata: %d routers, %d services, %d middlewares, all enabled\n", len(raw["routers"]), len(raw["services"]), len(raw["middlewares"]))
}
