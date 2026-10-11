// Purpose: guard D-0376: every PersistentVolumeClaim the chart renders carries
// helm.sh/resource-policy: keep, so helm uninstall (and an upgrade that drops a
// volume) never deletes the claim and, with a Delete reclaim policy, the data.
//
// Inputs: the embedded chart (Chart) and, when helm is on PATH or HELM_BIN is
// set, the rendered fixtures testdata/{full,interp}/golden.
//
// Outputs: test failures naming the template or rendered claim without the
// annotation.
//
// Constraints: the template check always runs (no helm needed). The render
// check skips without helm; tests/template-check.sh and the kind run render it
// on hosts that have helm.
package nselfk8s

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const keepAnnotation = "helm.sh/resource-policy"

// pvcDocsWithoutKeep returns the manifests in text (split on "---") that are a
// PersistentVolumeClaim whose metadata block lacks the keep annotation, and
// how many PersistentVolumeClaims it saw.
func pvcDocsWithoutKeep(text string) (missing []string, seen int) {
	for _, doc := range strings.Split(text, "\n---") {
		lines := strings.Split(doc, "\n")
		isPVC := false
		for _, l := range lines {
			if strings.TrimSpace(l) == "kind: PersistentVolumeClaim" && !strings.HasPrefix(l, " ") {
				isPVC = true
			}
		}
		if !isPVC {
			continue
		}
		seen++
		inMeta, keep := false, false
		for _, l := range lines {
			switch {
			case l == "metadata:":
				inMeta = true
			case inMeta && l != "" && !strings.HasPrefix(l, " "):
				inMeta = false
			case inMeta && strings.TrimSpace(l) == keepAnnotation+": keep":
				keep = true
			}
		}
		if !keep {
			missing = append(missing, doc)
		}
	}
	return missing, seen
}

func TestPVCTemplatesKeepClaims(t *testing.T) {
	templates := path.Join(ChartRoot, "templates")
	total := 0
	err := fs.WalkDir(Chart, templates, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(Chart, p)
		if err != nil {
			return err
		}
		missing, seen := pvcDocsWithoutKeep(string(b))
		total += seen
		for _, doc := range missing {
			t.Errorf("%s: PersistentVolumeClaim without %s: keep:\n%s", p, keepAnnotation, doc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("no PersistentVolumeClaim template found: the check would pass vacuously")
	}
}

func TestPVCDocsWithoutKeep(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		missing int
		seen    int
	}{
		{"kept", "kind: PersistentVolumeClaim\nmetadata:\n  name: a\n  annotations:\n    helm.sh/resource-policy: keep\nspec: {}\n", 0, 1},
		{"no annotations", "kind: PersistentVolumeClaim\nmetadata:\n  name: a\nspec: {}\n", 1, 1},
		{"annotation outside metadata", "kind: PersistentVolumeClaim\nmetadata:\n  name: a\nspec:\n  helm.sh/resource-policy: keep\n", 1, 1},
		{"other value", "kind: PersistentVolumeClaim\nmetadata:\n  annotations:\n    helm.sh/resource-policy: delete\n", 1, 1},
		{"second claim bare", "kind: PersistentVolumeClaim\nmetadata:\n  annotations:\n    helm.sh/resource-policy: keep\n---\nkind: PersistentVolumeClaim\nmetadata:\n  name: b\n", 1, 2},
		{"not a claim", "kind: Service\nmetadata:\n  name: a\n", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			missing, seen := pvcDocsWithoutKeep(c.text)
			if len(missing) != c.missing || seen != c.seen {
				t.Errorf("missing %d seen %d, want %d and %d", len(missing), seen, c.missing, c.seen)
			}
		})
	}
}

// TestRenderedPVCsKeepClaims renders the fixtures with helm and checks every
// rendered PersistentVolumeClaim. Skips without helm.
func TestRenderedPVCsKeepClaims(t *testing.T) {
	helm := os.Getenv("HELM_BIN")
	if helm == "" {
		p, err := exec.LookPath("helm")
		if err != nil {
			t.Skip("helm not on PATH and HELM_BIN unset")
		}
		helm = p
	}
	for _, fx := range []string{"full", "interp"} {
		g := path.Join("testdata", fx, "golden")
		out, err := exec.Command(helm, "template", "nself", ChartRoot,
			"-f", path.Join(g, "values.yaml"), "-f", path.Join(g, "secrets.yaml")).Output()
		if err != nil {
			t.Fatalf("%s: helm template: %v", fx, err)
		}
		seen := 0
		dec := yaml.NewDecoder(bytes.NewReader(out))
		for {
			var m struct {
				Kind     string `yaml:"kind"`
				Metadata struct {
					Name        string            `yaml:"name"`
					Annotations map[string]string `yaml:"annotations"`
				} `yaml:"metadata"`
			}
			if err := dec.Decode(&m); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatalf("%s: decode: %v", fx, err)
			}
			if m.Kind != "PersistentVolumeClaim" {
				continue
			}
			seen++
			if m.Metadata.Annotations[keepAnnotation] != "keep" {
				t.Errorf("%s: rendered claim %s lacks %s: keep", fx, m.Metadata.Name, keepAnnotation)
			}
		}
		if seen == 0 {
			t.Errorf("%s: rendered no PersistentVolumeClaim", fx)
		}
	}
}
