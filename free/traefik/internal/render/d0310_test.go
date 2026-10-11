package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// D-0310: routes.json carries four optional fields (upstream path and query, static_file,
// headers_add) and the core:api-docs route. api-docs is skipped; the rest is refused.

func fixtureJSON(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "http-full", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func renderDoc(t *testing.T, doc map[string]any) ([]byte, error) {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	m, err := contract.Load(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl"})
}

func docsLoc(path, file string) map[string]any {
	return map[string]any{"path": path, "match": "prefix", "static_file": file,
		"headers_add": map[string]any{"X-Example": "one"}}
}

func docsRoute() map[string]any {
	return map[string]any{"id": "core:api-docs", "source": "core", "file": "nginx/sites/api-docs.conf",
		"server_names": []string{"docs.example.test"}, "listen": map[string]any{"http": false, "https": true},
		"locations": []any{docsLoc("/api-docs", "/opt/example/openapi.json"), docsLoc("/docs", "/opt/example/scalar.html")}}
}

func firstLoc(doc map[string]any) map[string]any {
	return doc["routes"].([]any)[0].(map[string]any)["locations"].([]any)[0].(map[string]any)
}

func TestApiDocsRouteIsSkippedNotRefused(t *testing.T) {
	want, err := renderDoc(t, fixtureJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	doc := fixtureJSON(t)
	doc["routes"] = append(doc["routes"].([]any), docsRoute())
	got, err := renderDoc(t, doc)
	if err != nil {
		t.Fatalf("api-docs must not be refused: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("the api-docs route changed the output; it must be skipped entirely")
	}
	if strings.Contains(string(got), "docs.example.test") || strings.Contains(string(got), "api-docs") {
		t.Error("the docs host is named in the output")
	}
}

func TestOptionalFieldsAreRefused(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, c := range []struct {
		name, field string
		mut         func(loc map[string]any)
	}{
		{"static_file", ".static_file", func(l map[string]any) { l["static_file"] = "/opt/example/x.json" }},
		{"headers_add", ".headers_add", func(l map[string]any) { l["headers_add"] = map[string]any{"X-Example": "one"} }},
		{"path", ".upstream.path", func(l map[string]any) { l["upstream"].(map[string]any)["path"] = "/x" }},
		{"path slash", ".upstream.path", func(l map[string]any) { l["upstream"].(map[string]any)["path"] = "/" }},
		{"query", ".upstream.query", func(l map[string]any) { l["upstream"].(map[string]any)["query"] = *str("a=b") }},
		{"bare query", ".upstream.query", func(l map[string]any) { l["upstream"].(map[string]any)["query"] = *str("") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := fixtureJSON(t)
			c.mut(firstLoc(doc))
			_, err := renderDoc(t, doc)
			var re *RefusalError
			if !errors.As(err, &re) {
				t.Fatalf("want a refusal, got %v", err)
			}
			found := false
			for _, it := range re.Items {
				if it.Route == "core:auth" && strings.HasSuffix(it.Field, c.field) {
					found = true
				}
			}
			if !found {
				t.Errorf("no refusal ending in %s: %v", c.field, err)
			}
		})
	}
}

// The skip is for the real shape only: the id with a location that is not a single-file alias,
// or another id carrying static_file, is still rendered or refused.
func TestApiDocsSkipIsNarrow(t *testing.T) {
	doc := fixtureJSON(t)
	r := docsRoute()
	r["locations"].([]any)[1].(map[string]any)["static_file"] = nil
	r["locations"].([]any)[1].(map[string]any)["headers_add"] = nil
	doc["routes"] = append(doc["routes"].([]any), r)
	if _, err := renderDoc(t, doc); err == nil {
		t.Error("a docs route with a non-static location must not be skipped")
	}
	doc = fixtureJSON(t)
	r = docsRoute()
	r["id"] = "core:other-docs"
	doc["routes"] = append(doc["routes"].([]any), r)
	if _, err := renderDoc(t, doc); err == nil {
		t.Error("static_file on another route id must be refused")
	}
}
