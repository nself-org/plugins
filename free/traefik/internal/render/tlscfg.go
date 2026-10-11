package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// goSuites maps OpenSSL cipher names to the Go names Traefik accepts. DHE suites
// are not implemented by crypto/tls and are omitted (the ECDHE suites remain).
var goSuites = map[string]string{
	"ECDHE-ECDSA-AES128-GCM-SHA256": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	"ECDHE-RSA-AES128-GCM-SHA256":   "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	"ECDHE-ECDSA-AES256-GCM-SHA384": "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	"ECDHE-RSA-AES256-GCM-SHA384":   "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	"ECDHE-ECDSA-CHACHA20-POLY1305": "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
	"ECDHE-RSA-CHACHA20-POLY1305":   "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	"ECDHE-ECDSA-AES128-SHA256":     "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
	"ECDHE-RSA-AES128-SHA256":       "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
}

var versions = map[string]string{"TLSv1.2": "VersionTLS12", "TLSv1.3": "VersionTLS13"}
var dirRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// tlsOptions builds a Traefik TLS option from nginx protocols and ciphers and
// returns its name ("" for the default option, "<hash>" otherwise).
func (b *builder) tlsOptions(route string, protocols []string, ciphers *string) (string, bool) {
	spec, ok := b.tlsSpec(route, protocols, ciphers)
	if !ok {
		return "", false
	}
	def, _ := b.tlsSpec("-", b.m.Defaults.TLS.Protocols, b.m.Defaults.TLS.Ciphers)
	b.tlsOp["default"] = def
	if h8(spec) == h8(def) {
		return "", true
	}
	name := "opt-" + h8(spec)
	b.tlsOp[name] = spec
	return name, true
}

func (b *builder) tlsSpec(route string, protocols []string, ciphers *string) (map[string]any, bool) {
	spec := map[string]any{}
	if len(protocols) == 0 {
		protocols = []string{"TLSv1.2", "TLSv1.3"}
	}
	var vs []string
	for _, p := range protocols {
		v, ok := versions[p]
		if !ok {
			b.refuse(route, "tls.protocols", fmt.Sprintf("protocol %q is not TLSv1.2 or TLSv1.3", p))
			return nil, false
		}
		vs = append(vs, v)
	}
	sort.Strings(vs)
	spec["minVersion"] = vs[0]
	spec["maxVersion"] = vs[len(vs)-1]
	if ciphers != nil && *ciphers != "" {
		var suites []string
		for _, c := range strings.Split(*ciphers, ":") {
			if g, ok := goSuites[c]; ok {
				suites = append(suites, g)
			} else if !strings.HasPrefix(c, "DHE-") {
				b.refuse(route, "tls.ciphers", fmt.Sprintf("cipher %q has no Go equivalent", c))
				return nil, false
			}
		}
		if len(suites) == 0 {
			b.refuse(route, "tls.ciphers", fmt.Sprintf("%q leaves no cipher Go implements; Go would fall back to its broader defaults", *ciphers))
			return nil, false
		}
		spec["cipherSuites"] = suites
	}
	return spec, true
}

// tlsFor registers the route's certificate lineage and TLS option.
func (b *builder) tlsFor(r contract.Route) (string, bool) {
	if r.TLS == nil {
		b.refuse(r.ID, "tls", "listen.https is true but tls is null")
		return "", false
	}
	b.hasTLS = true
	if !b.cert(r.ID, r.TLS.SSLDir) {
		return "", false
	}
	return b.tlsOptions(r.ID, r.TLS.Protocols, r.TLS.Ciphers)
}

// cert resolves ssl/certificates/<dir> to its generation directory (P7-LIVE-15:
// <dir> is a link to .<dir>.gen-<n>), so a renewal changes the parsed config.
func (b *builder) cert(route, dir string) bool {
	if _, done := b.certs[dir]; done {
		return true
	}
	if !dirRE.MatchString(dir) {
		b.refuse(route, "tls.ssl_dir", fmt.Sprintf("%q is not a plain directory name", dir))
		return false
	}
	base := filepath.Join(b.o.SSLRoot, "certificates")
	link := filepath.Join(base, dir)
	resolved := dir
	if fi, err := os.Lstat(link); err != nil {
		b.refuse(route, "tls.ssl_dir", fmt.Sprintf("certificate lineage %q not found under %s", dir, base))
		return false
	} else if fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(link)
		if err != nil || filepath.Base(t) != filepath.Clean(t) || t == "." || t == ".." {
			b.refuse(route, "tls.ssl_dir", fmt.Sprintf("lineage link %q must point at a sibling generation directory", dir))
			return false
		}
		resolved = t
	}
	for _, f := range []string{"fullchain.pem", "privkey.pem"} {
		if _, err := os.Stat(filepath.Join(base, resolved, f)); err != nil {
			b.refuse(route, "tls.ssl_dir", fmt.Sprintf("%s missing in %s", f, resolved))
			return false
		}
	}
	root := filepath.Join(b.o.CertRoot, "certificates", resolved)
	b.certs[dir] = [2]string{filepath.Join(root, "fullchain.pem"), filepath.Join(root, "privkey.pem")}
	return true
}
