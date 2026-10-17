package render

import (
	"strings"
	"testing"
)

// D-0598: the real fixture must render both docs locations through the helper.
func TestAPIDocsServed(t *testing.T) {
	out, err := renderDoc(t, fixtureJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Host(`docs.example.test`)", "PathPrefix(`/api-docs`)", "PathPrefix(`/docs`)", "/__nself/docs/openapi.json", "/__nself/docs/scalar.html", "Access-Control-Allow-Origin", "no-store"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("docs route missing %q", want)
		}
	}
}

func TestAPIDocsRejectsUnsupportedInput(t *testing.T) {
	for _, kind := range []string{"file", "header", "variable", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			doc := fixtureJSON(t)
			routes := doc["routes"].([]any)
			l := routes[len(routes)-1].(map[string]any)["locations"].([]any)[0].(map[string]any)
			switch kind {
			case "file":
				l["static_file"] = "/opt/nself/.dist/secret"
			case "header":
				l["headers_add"] = map[string]string{"Bad\nName": "value"}
			case "variable":
				l["headers_add"] = map[string]string{"X-Test": "$host"}
			case "mixed":
				l["deny_all"] = true
			}
			if _, err := renderDoc(t, doc); err == nil {
				t.Fatal("unsupported docs input was accepted")
			}
		})
	}
}
