package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type pricingRoundTripper http.HandlerFunc

func (f pricingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body == nil {
		r.Body = http.NoBody
	}
	w := httptest.NewRecorder()
	f(w, r)
	return w.Result(), nil
}

func withPricingServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	oldBase, oldClient := hetznerAPIBaseURL, hetznerHTTPClient
	hetznerAPIBaseURL = "http://127.0.0.1/v1"
	hetznerHTTPClient = &http.Client{Transport: pricingRoundTripper(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
		handler(w, r)
	})}
	t.Cleanup(func() { hetznerAPIBaseURL, hetznerHTTPClient = oldBase, oldClient })
}

func TestHetznerPricing(t *testing.T) {
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/pricing" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		// API v1 response shape, with different net and gross values.
		_, _ = io.WriteString(w, `{"pricing":{"currency":"EUR","vat_rate":"19.00",
		"server_types":[{"id":22,"name":"cx22","prices":[
		{"location":"fsn1","price_hourly":{"net":"0.0060","gross":"0.00714"},"price_monthly":{"net":"3.79","gross":"4.5101"}},
		{"location":"ash","price_hourly":{"net":"0.0080","gross":"0.00952"},"price_monthly":{"net":"4.99","gross":"5.9381"}}]}],
		"primary_ips":[{"type":"ipv4","prices":[{"location":"fsn1","price_hourly":{"net":"0.0008","gross":"0.000952"},"price_monthly":{"net":"0.50","gross":"0.595"}}]}],
		"server_backup":{"percentage":"20.00"}}}`)
	})
	p, err := NewHetznerClient("").GetPricing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Currency != "EUR" || p.BackupPercentage != 20 || len(p.ServerTypes) != 1 || len(p.PrimaryIPs) != 1 {
		t.Fatalf("unexpected pricing: %+v", p)
	}
	st := p.ServerTypes[0]
	if st.Name != "cx22" || st.ID != 22 || len(st.Prices) != 2 {
		t.Fatalf("unexpected server pricing: %+v", st)
	}
	if st.Prices[0] != (LocationPrice{"fsn1", 0.00714, 4.5101}) || st.Prices[1] != (LocationPrice{"ash", 0.00952, 5.9381}) {
		t.Fatalf("gross location prices: %+v", st.Prices)
	}
	if p.PrimaryIPs[0].Type != "ipv4" || p.PrimaryIPs[0].Prices[0] != (LocationPrice{"fsn1", 0.000952, 0.595}) {
		t.Fatalf("primary IP pricing: %+v", p.PrimaryIPs)
	}
}

func TestHetznerPricingInvalid(t *testing.T) {
	for _, body := range []string{
		`{"pricing":{"currency":"USD","server_backup":{"percentage":"20"}}}`,
		`{"pricing":{"currency":"EUR"}}`,
		`{"pricing":{"currency":"EUR","server_backup":{"percentage":"20"},"server_types":[{"prices":[{"price_hourly":{"gross":"1"},"price_monthly":{"gross":"1"}}]}]}}`,
		`{"pricing":{"currency":"EUR","server_backup":{"percentage":"20"},"server_types":[{"prices":[{"price_hourly":{"gross":"-1"},"price_monthly":{"gross":"1"}}]}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			withPricingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := NewHetznerClient("").GetPricing(context.Background()); err == nil {
				t.Fatal("expected invalid pricing error")
			}
		})
	}
}

func TestHetznerScopeProbe(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
		scope  string
	}{
		{"invalid_input", 422, "read-write"}, {"invalid_input", 400, "read-write"}, {"json_error", 400, "read-write"},
		{"json_error", 500, ""}, {"json_error", 403, ""},
		{"forbidden", 403, "read-only"}, {"unauthorized", 401, ""},
		{"rate_limit_exceeded", 429, ""}, {"invalid_input", 500, ""},
		{"", 200, ""}, {"", 204, ""}, {"", 201, ""},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			calls := 0
			withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.URL.Path != "/ssh_keys" || len(body) != 0 {
					t.Errorf("unexpected probe request: %s %s (%d bytes)", r.Method, r.URL.Path, len(body))
				}
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": tc.code}})
			})
			scope, err := NewHetznerClient("").ProbeWriteScope(context.Background())
			if scope != tc.scope || (err != nil) != (tc.scope == "") || calls != 1 {
				t.Fatalf("scope=%q error=%v calls=%d", scope, err, calls)
			}
		})
	}
}

func TestHetznerScopeProbeUnexpectedCreation(t *testing.T) {
	for _, cleanupStatus := range []int{204, 500} {
		t.Run(fmt.Sprint(cleanupStatus), func(t *testing.T) {
			deleted := false
			withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST /ssh_keys":
					w.WriteHeader(201)
					_, _ = io.WriteString(w, `{"ssh_key":{"id":42}}`)
				case "DELETE /ssh_keys/42":
					deleted = true
					w.WriteHeader(cleanupStatus)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})
			scope, err := NewHetznerClient("").ProbeWriteScope(context.Background())
			if err == nil || scope != "" || !deleted {
				t.Fatalf("scope=%q error=%v deleted=%v", scope, err, deleted)
			}
			if cleanupStatus == 500 && !strings.Contains(err.Error(), "cleanup failed") {
				t.Fatal("cleanup failure not reported")
			}
		})
	}
}

func TestHetznerBaseOverride(t *testing.T) {
	for _, base := range []string{"https://example.com/v1", "http://127.0.0.2", "http://localhost.example.com", "http://[::ffff:127.0.0.1]", "http://localhost@remote.test", "file://localhost/v1", "http://localhost/?x=1", "://"} {
		t.Setenv("NSELF_INFRA_API_BASE", base)
		if _, err := apiBaseURL(); err == nil {
			t.Fatalf("accepted unsafe override %q", base)
		}
		if _, err := NewHetznerClient("").GetPricing(context.Background()); err == nil {
			t.Fatal("unsafe override reached transport")
		}
	}
	for _, base := range []string{"http://127.0.0.1:1234/v1", "http://[::1]:1234/v1", "http://localhost:1234/v1"} {
		t.Setenv("NSELF_INFRA_API_BASE", base)
		if got, err := apiBaseURL(); err != nil || got != base {
			t.Fatalf("loopback override: got %q, error %v", got, err)
		}
	}
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "localhost:1234" || r.URL.Path != "/locations" {
			t.Errorf("unexpected override path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"locations":[]}`)
	})
	t.Setenv("NSELF_INFRA_API_BASE", "http://localhost:1234/v1")
	if _, err := NewHetznerClient("").ListLocations(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHetznerPricingRelatedCalls(t *testing.T) {
	selector := "project in (one,two),name=a&b+c"
	deleted := false
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /locations":
			_, _ = io.WriteString(w, `{"locations":[{"id":1,"name":"fsn1","country":"DE"}]}`)
		case "GET /datacenters":
			_, _ = io.WriteString(w, `{"datacenters":[{"id":1,"name":"fsn1-dc14","location":{"name":"fsn1"},"server_types":{"supported":[22,23],"available":[22]}}]}`)
		case "GET /servers", "GET /ssh_keys":
			if r.URL.Query().Get("label_selector") != selector || len(r.URL.Query()["label_selector"]) != 1 {
				t.Errorf("selector not preserved: %s", r.URL.RawQuery)
			}
			if r.URL.Path == "/servers" {
				_, _ = io.WriteString(w, `{"servers":[]}`)
			} else if r.URL.Query().Get("page") == "1" {
				_, _ = io.WriteString(w, `{"ssh_keys":[{"id":1}],"meta":{"pagination":{"next_page":2}}}`)
			} else {
				_, _ = io.WriteString(w, `{"ssh_keys":[{"id":2}],"meta":{"pagination":{"next_page":null}}}`)
			}
		case "POST /ssh_keys":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["name"] != "journey" || body["labels"] == nil {
				t.Error("missing key metadata")
			}
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"ssh_key":{"id":3,"name":"journey","labels":{"project":"one"}}}`)
		case "DELETE /ssh_keys/3":
			deleted = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	c, ctx := NewHetznerClient(""), context.Background()
	locations, err := c.ListLocations(ctx)
	if err != nil || len(locations) != 1 || locations[0].Name != "fsn1" {
		t.Fatalf("locations=%+v error=%v", locations, err)
	}
	dcs, err := c.ListDatacenters(ctx)
	if err != nil || len(dcs) != 1 || len(dcs[0].ServerTypes.Available) != 1 || dcs[0].ServerTypes.Available[0] != 22 {
		t.Fatalf("datacenters=%+v error=%v", dcs, err)
	}
	if _, err := c.ListServers(ctx, ListOptions{LabelSelector: selector}); err != nil {
		t.Fatal(err)
	}
	keys, err := c.ListSSHKeysByLabel(ctx, selector)
	if err != nil || len(keys) != 2 || keys[1].ID != 2 {
		t.Fatalf("keys=%+v error=%v", keys, err)
	}
	key, err := c.CreateSSHKey(ctx, "journey", "", map[string]string{"project": "one"})
	if err != nil || key.ID != 3 || key.Labels["project"] != "one" {
		t.Fatalf("key=%+v error=%v", key, err)
	}
	if err := c.DeleteSSHKey(ctx, key.ID); err != nil || !deleted {
		t.Fatalf("delete error=%v deleted=%v", err, deleted)
	}
}

func TestHetznerScopeProbeRedactsErrors(t *testing.T) {
	c := &hetznerClient{token: t.Name()}
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": "unauthorized", "message": c.token}})
	})
	_, err := c.ProbeWriteScope(context.Background())
	if err == nil || strings.Contains(err.Error(), c.token) {
		t.Fatal("error must exist and must not contain authentication material")
	}
}

func TestHetznerBaseOverrideRedirect(t *testing.T) {
	calls := 0
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "https://example.com/pricing")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	t.Setenv("NSELF_INFRA_API_BASE", "http://localhost/v1")
	if _, err := NewHetznerClient("").GetPricing(context.Background()); err == nil || calls != 1 {
		t.Fatalf("redirect must fail without another request; calls=%d error=%v", calls, err)
	}
}

func TestHetznerScopeProbeCleanupAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deleted := false
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			cancel()
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"ssh_key":{"id":42}}`)
			return
		}
		if r.Method != "DELETE" || r.URL.Path != "/ssh_keys/42" || r.Context().Err() != nil {
			t.Error("cleanup must use a valid context and the created key ID")
		}
		deleted = true
		w.WriteHeader(204)
	})
	if scope, err := NewHetznerClient("").ProbeWriteScope(ctx); err == nil || scope != "" || !deleted {
		t.Fatalf("scope=%q error=%v deleted=%v", scope, err, deleted)
	}
}

func TestHetznerListServersLabelSelectorEncoded(t *testing.T) {
	var got string
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		if r.URL.Query().Get("label_selector") != "a=b,c&d=e" {
			t.Errorf("selector not preserved as one value: %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"servers":[]}`)
	})
	if _, err := NewHetznerClient("").ListServers(context.Background(), ListOptions{LabelSelector: "a=b,c&d=e"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "&d=e") {
		t.Fatalf("selector leaked into a separate parameter: %q", got)
	}
}

func TestHetznerErrorNamesCallWithoutQuery(t *testing.T) {
	withPricingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": "not_found", "message": "gone"}})
	})
	err := NewHetznerClient("").DeleteServer(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "DELETE /servers/1") || !strings.Contains(err.Error(), "404 not_found") {
		t.Fatalf("error lacks call context: %v", err)
	}
	_, err = NewHetznerClient("").ListServers(context.Background(), ListOptions{LabelSelector: "secret=1"})
	if err == nil || strings.Contains(err.Error(), "secret=1") {
		t.Fatalf("query string must not reach error text: %v", err)
	}
}
