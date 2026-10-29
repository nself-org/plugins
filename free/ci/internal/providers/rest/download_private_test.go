package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadPrivateBase(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, allow := range []bool{false, true} {
			t.Run(provider+"/"+map[bool]string{false: "denied", true: "allowed"}[allow], func(t *testing.T) {
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/redirect" {
						http.Redirect(w, r, "/artifact", http.StatusFound)
						return
					}
					_, _ = w.Write([]byte("artifact"))
				}))
				defer s.Close()
				c := testClient(s)
				c.TLSConfig = c.TLSConfig.Clone()
				c.TLSConfig.ServerName = "127.0.0.1"
				c.BaseURL = strings.Replace(s.URL, "127.0.0.1", "localhost", 1)
				c.Provider, c.AllowPrivateBase = provider, allow
				c.RedirectHosts = []string{"localhost"}
				var out bytes.Buffer
				_, err := c.Download(context.Background(), c.BaseURL+"/artifact", 100, &out)
				if allow {
					if err != nil || out.String() != "artifact" {
						t.Fatalf("download: %v, bytes=%d", err, out.Len())
					}
				} else {
					assertError(t, err, "E702", "ssrf.private")
				}
				_, err = c.Download(context.Background(), c.BaseURL+"/redirect", 100, &out)
				assertError(t, err, "E702", "ssrf.private")
			})
		}
	}
}
