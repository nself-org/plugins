package contract

import (
	"bytes"
	"encoding/json"
	"testing"
)

// docsRoute is the core:api-docs route the cli writes for every project with api docs on.
func docsRoute() map[string]any {
	loc := func(path, file string) map[string]any {
		return map[string]any{"path": path, "match": "prefix", "static_file": file,
			"headers_add": map[string]any{"X-Example": "one"}}
	}
	return map[string]any{"id": "core:api-docs", "source": "core", "file": "nginx/sites/api-docs.conf",
		"server_names": []string{"docs.example.test"}, "listen": map[string]any{"http": false, "https": true},
		"locations": []any{loc("/api-docs", "/opt/example/openapi.json"), loc("/docs", "/opt/example/scalar.html")}}
}

func withRoute(t *testing.T, mut func(doc map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(fixtureBytes(t), &doc); err != nil {
		t.Fatal(err)
	}
	mut(doc)
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadAcceptsTheOptionalFields(t *testing.T) {
	b := withRoute(t, func(doc map[string]any) {
		routes := doc["routes"].([]any)
		first := routes[0].(map[string]any)["locations"].([]any)[0].(map[string]any)
		first["upstream"].(map[string]any)["path"] = "/x/"
		first["upstream"].(map[string]any)["query"] = ""
		doc["routes"] = append(routes, docsRoute())
	})
	m, err := Load(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	up := m.Routes[0].Locations[0].Upstream
	if up.Path != "/x/" || up.Query == nil || *up.Query != "" {
		t.Errorf("upstream path/query not decoded: %+v", up)
	}
	docs := m.Routes[len(m.Routes)-1]
	if docs.ID != "core:api-docs" || len(docs.Locations) != 2 {
		t.Fatalf("docs route not decoded: %+v", docs)
	}
	l := docs.Locations[1]
	if l.StaticFile == nil || *l.StaticFile != "/opt/example/scalar.html" || l.HeadersAdd["X-Example"] != "one" || l.Upstream != nil {
		t.Errorf("static_file/headers_add not decoded: %+v", l)
	}
}

func TestLoadStillRejectsUnknownFieldsNextToTheNewOnes(t *testing.T) {
	for _, key := range []string{"static_files", "headers_added", "future_field"} {
		b := withRoute(t, func(doc map[string]any) {
			r := docsRoute()
			r["locations"].([]any)[0].(map[string]any)[key] = "x"
			doc["routes"] = append(doc["routes"].([]any), r)
		})
		if m, err := Load(bytes.NewReader(b)); err == nil || m != nil {
			t.Errorf("%s: want an error, got %v", key, err)
		}
	}
	b := withRoute(t, func(doc map[string]any) {
		doc["routes"].([]any)[0].(map[string]any)["locations"].([]any)[0].(map[string]any)["upstream"].(map[string]any)["extra"] = 1
	})
	if _, err := Load(bytes.NewReader(b)); err == nil {
		t.Error("unknown upstream field was accepted")
	}
}
