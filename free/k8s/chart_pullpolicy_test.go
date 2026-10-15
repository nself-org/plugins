// Purpose: verify D-0618 pull policies survive rendering the shared pod helper.
// Inputs: the embedded chart and explicit or omitted pull policies.
// Outputs: rendered pod YAML assertions.
// Constraints: no external Helm binary or cluster required.
package nselfk8s

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Render the shared pod helper directly: all workload templates use this helper.
// Unused Helm functions are registered but fail if a fixture starts using them.
func TestPodRendersImagePullPolicy(t *testing.T) {
	b, err := fs.ReadFile(Chart, ChartRoot+"/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	var tmpl *template.Template
	funcs := template.FuncMap{
		"dict": func(args ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i < len(args); i += 2 {
				m[args[i].(string)] = args[i+1]
			}
			return m
		},
		"default": func(fallback, value any) any {
			if value == nil || value == "" {
				return fallback
			}
			return value
		},
		"hasPrefix": func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"quote":     func(value any) string { return fmt.Sprintf("%q", value) },
		"include": func(name string, data any) (string, error) {
			var out bytes.Buffer
			err := tmpl.ExecuteTemplate(&out, name, data)
			return out.String(), err
		},
	}
	for _, name := range []string{"lower", "hasSuffix", "trimSuffix", "toJson", "toYaml", "nindent", "hasKey", "set"} {
		funcs[name] = func(...any) (string, error) { return "", fmt.Errorf("unexpected Helm function") }
	}
	tmpl, err = template.New("helpers").Funcs(funcs).Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"Always", "IfNotPresent", "Never", ""} {
		t.Run(policy, func(t *testing.T) {
			var out bytes.Buffer
			err := tmpl.ExecuteTemplate(&out, "nself.pod", map[string]any{
				"root": map[string]any{"Values": map[string]any{}},
				"name": "app", "restart": "Always",
				"svc": map[string]any{"image": "nginx", "tag": "1.27", "pullPolicy": policy},
			})
			if err != nil {
				t.Fatal(err)
			}
			var pod struct {
				Containers []struct {
					ImagePullPolicy string `yaml:"imagePullPolicy"`
				} `yaml:"containers"`
			}
			if err := yaml.Unmarshal(out.Bytes(), &pod); err != nil {
				t.Fatal(err)
			}
			if len(pod.Containers) != 1 || pod.Containers[0].ImagePullPolicy != policy {
				t.Fatalf("pull policy %q lost in rendered pod:\n%s", policy, out.String())
			}
		})
	}
}
