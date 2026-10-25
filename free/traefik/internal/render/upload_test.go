package render

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// renderUpload renders a fixture with ids named for the upload entrypoint.
func renderUpload(t *testing.T, name string, ids ...string) ([]byte, error) {
	t.Helper()
	m := fixture(t, name)
	return Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl", Upload: ids})
}

func routersOf(t *testing.T, out []byte) map[string]map[string]any {
	t.Helper()
	got := map[string]map[string]any{}
	for n, v := range httpSection(t, out)["routers"] {
		got[n] = v.(map[string]any)
	}
	return got
}

// A named route is served twice: every router has a -upl twin on the upload entrypoint with the
// same rule, priority, middlewares, service and tls; no other route and no default router has one.
func TestNamedRouteIsMirroredOnTheUploadEntrypoint(t *testing.T) {
	for _, fx := range []string{"http-full", "tls-full"} {
		const id = "core:storage"
		out, err := renderUpload(t, fx, id)
		if err != nil {
			t.Fatal(err)
		}
		rs := routersOf(t, out)
		pfx := rname(id) + "-"
		var twins int
		for n, spec := range rs {
			if !strings.HasSuffix(n, "-upl") {
				if strings.HasPrefix(n, pfx) {
					up, ok := rs[n+"-upl"]
					if !ok {
						t.Errorf("%s: router %s has no -upl twin", fx, n)
						continue
					}
					twins++
					for _, k := range []string{"rule", "priority", "middlewares", "service", "tls"} {
						if !reflect.DeepEqual(spec[k], up[k]) {
							t.Errorf("%s: %s %s = %v, twin has %v", fx, n, k, spec[k], up[k])
						}
					}
					if !reflect.DeepEqual(up["entryPoints"], []any{"upload"}) {
						t.Errorf("%s: %s-upl entryPoints = %v", fx, n, up["entryPoints"])
					}
				}
				continue
			}
			if !strings.HasPrefix(n, pfx) {
				t.Errorf("%s: router %s belongs to a route that was not named", fx, n)
			}
		}
		if twins < 3 {
			t.Errorf("%s: only %d routers mirrored", fx, twins)
		}
		// without the option nothing is mirrored
		plain, err := renderUpload(t, fx)
		if err != nil {
			t.Fatal(err)
		}
		for n := range routersOf(t, plain) {
			if strings.HasSuffix(n, "-upl") {
				t.Errorf("%s: %s rendered without --upload-routes", fx, n)
			}
		}
	}
}

// A route on both listeners mirrors its secure one only: the plain router keeps no twin.
func TestUploadMirrorsTheSecureListenerWhenThereIsOne(t *testing.T) {
	m := fixture(t, "tls-full")
	for i := range m.Routes {
		if m.Routes[i].ID == "core:auth" {
			m.Routes[i].Listen.HTTP = true
		}
	}
	out, err := Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl", Upload: []string{"core:auth"}})
	if err != nil {
		t.Fatal(err)
	}
	rs := routersOf(t, out)
	l0 := rname("core:auth") + "-l0"
	if rs[l0+"-web"] == nil || rs[l0+"-tls"] == nil || rs[l0+"-tls-upl"] == nil {
		t.Fatalf("missing routers among %s-web, -tls, -tls-upl", l0)
	}
	if rs[l0+"-web-upl"] != nil {
		t.Error("the plain listener must not be mirrored when the route has a secure one")
	}
	if rs[l0+"-tls-upl"]["tls"] == nil {
		t.Error("the mirror of a secure listener must terminate TLS")
	}
}

func TestUnknownUploadRouteIsRefused(t *testing.T) {
	_, err := renderUpload(t, "http-full", "core:storage", "nope:1")
	var ref *RefusalError
	if !errors.As(err, &ref) || len(ref.Items) != 1 || ref.Items[0].Route != "-" || ref.Items[0].Field != "upload_routes" || ref.Items[0].Reason != "no route nope:1" {
		t.Fatalf("want one upload_routes refusal for nope:1, got %v", err)
	}
}

// The upload listener mirrors only the named routes, so a named wildcard would answer, with its own
// upstream, a host that an unnamed route owns on web and websecure (D-0617). That is refused.
func TestUploadWildcardOverlappingAnUnnamedRouteIsRefused(t *testing.T) {
	const wild = "plugin:wild/wild-wild.conf#1"
	_, err := renderUpload(t, "host-tier", wild)
	var ref *RefusalError
	if !errors.As(err, &ref) {
		t.Fatalf("want a refusal, got %v", err)
	}
	var got []string
	for _, it := range ref.Items {
		if it.Route != wild || it.Field != "upload_routes" {
			t.Errorf("unexpected refusal %+v", it)
		}
		got = append(got, it.Reason)
	}
	want := "unnamed route plugin:notes/notes.conf#1 owns notes.example.test, which this wildcard would answer on the upload port"
	found := false
	for _, g := range got {
		found = found || g == want
	}
	if !found {
		t.Fatalf("missing %q in %q", want, got)
	}
}

// Naming the overlapping routes too (or a wildcard with nothing under it) renders.
func TestUploadWildcardWithEveryOverlapNamedRenders(t *testing.T) {
	m := fixture(t, "host-tier")
	var ids []string
	for _, r := range m.Routes {
		ids = append(ids, r.ID)
	}
	if _, err := Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl", Upload: ids}); err != nil {
		t.Fatalf("all routes named: %v", err)
	}
	// a more specific wildcard owned by an unnamed route also overlaps
	m = fixture(t, "host-tier")
	deep := noteRoute(t, m)
	deep.ID, deep.ServerNames = "plugin:deep/deep.conf#1", []string{"*.deep.example.test"}
	m.Routes = append(m.Routes, deep)
	_, err := Render(m, Options{Upload: []string{"plugin:wild/wild-wild.conf#1", "core:auth", "core:hasura", "core:storage", "core:storage-console", "plugin:notes/notes.conf#1", "core:api-docs"}})
	var ref *RefusalError
	if !errors.As(err, &ref) || len(ref.Items) != 1 || !strings.Contains(ref.Items[0].Reason, "*.deep.example.test") {
		t.Fatalf("want one refusal naming *.deep.example.test, got %v", err)
	}
	// a wildcard on an unrelated suffix does not overlap
	m = fixture(t, "host-tier")
	m.Routes = append(m.Routes, deep)
	m.Routes[len(m.Routes)-1].ServerNames = []string{"*.other.test"}
	if _, err := Render(m, Options{Upload: []string{"core:auth"}}); err != nil {
		t.Fatalf("exact route named, unrelated wildcard unnamed: %v", err)
	}
}
