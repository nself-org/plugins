// Purpose: guard plugin-to-plugin DNS aliases (D-0641) from compose through Helm.
// Inputs: resolved compose workloads and the chart; helm on PATH or HELM_BIN.
// Outputs: failures for missing aliases, wrong routing, duplicate names or collisions.
// Constraints: no cluster required; the kind plugin-alias.sh proof checks real DNS/HTTP.
package nselfk8s

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/nself-k8s/internal/values"
	"gopkg.in/yaml.v3"
)

type renderedService struct {
	Kind     string
	Metadata struct{ Name string }
	Spec     struct {
		ClusterIP string `yaml:"clusterIP"`
		Selector  map[string]string
		Ports     []map[string]interface{}
	}
}

func renderPluginServices(t *testing.T, services map[string]values.ComposeSvc) ([]byte, error) {
	t.Helper()
	helm := os.Getenv("HELM_BIN")
	if helm == "" {
		var err error
		helm, err = exec.LookPath("helm")
		if err != nil {
			t.Skip("helm not on PATH and HELM_BIN unset")
		}
	}
	v, _, err := values.Map(&values.Model{Name: "aliases", Services: services}, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(helm, "template", "aliases", ChartRoot, "-f", "-")
	cmd.Stdin = bytes.NewReader(input)
	return cmd.CombinedOutput()
}

func aliasWorkload(host string) values.ComposeSvc {
	return values.ComposeSvc{Image: "busybox:1.37.0", Hostname: host,
		Ports: []values.ComposePort{{Target: 8080, Protocol: "tcp"}, {Target: 8125, Protocol: "udp"}}}
}

func TestPluginHostnamesRenderServices(t *testing.T) {
	job, noPorts := aliasWorkload("plugin-batch"), aliasWorkload("plugin-empty")
	job.Restart, noPorts.Ports = "no", nil
	stateful := aliasWorkload("plugin-store")
	stateful.Image = "postgres:16"
	out, err := renderPluginServices(t, map[string]values.ComposeSvc{
		"notify": aliasWorkload("plugin-notify"), "plugin-analytics": aliasWorkload("plugin-analytics"),
		"core": aliasWorkload("core"), "batch": job, "empty": noPorts,
		"nginx": aliasWorkload("plugin-nginx"), "store": stateful,
	})
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	services := map[string]renderedService{}
	decoder := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		var header struct{ Kind string }
		if err := node.Decode(&header); err != nil {
			t.Fatal(err)
		}
		if header.Kind == "Service" {
			var doc renderedService
			if err := node.Decode(&doc); err != nil {
				t.Fatal(err)
			}
			if _, exists := services[doc.Metadata.Name]; exists {
				t.Fatalf("duplicate Service %s", doc.Metadata.Name)
			}
			services[doc.Metadata.Name] = doc
		}
	}
	for _, name := range []string{"notify", "store"} {
		primary, ok := services[name]
		if !ok {
			t.Fatalf("missing original Service %s", name)
		}
		alias, ok := services["plugin-"+name]
		if !ok {
			t.Fatalf("missing Service plugin-%s for compose hostname", name)
		}
		if !reflect.DeepEqual(primary.Spec.Selector, alias.Spec.Selector) ||
			!reflect.DeepEqual(primary.Spec.Ports, alias.Spec.Ports) ||
			alias.Spec.Selector["app.kubernetes.io/name"] != name {
			t.Errorf("plugin-%s must select and expose the original workload: %+v", name, alias.Spec)
		}
	}
	for _, name := range []string{"plugin-batch", "plugin-empty", "plugin-nginx", "plugin-core"} {
		if _, exists := services[name]; exists {
			t.Errorf("unexpected Service %s", name)
		}
	}
	if len(services) != 6 {
		t.Errorf("got %d Services, want 6 (originals and distinct aliases)", len(services))
	}
}

func TestPluginServiceAliasCollisionsFail(t *testing.T) {
	for _, name := range []string{"plugin-notify", "other"} {
		t.Run(name, func(t *testing.T) {
			out, err := renderPluginServices(t, map[string]values.ComposeSvc{
				"notify": aliasWorkload("plugin-notify"), name: aliasWorkload("plugin-notify"),
			})
			if err == nil || !strings.Contains(string(out), "plugin Service alias plugin-notify") {
				t.Fatalf("want explicit alias collision error, got %v\n%s", err, out)
			}
		})
	}
}
