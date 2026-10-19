package toolchain

// Purpose: structural checks of .github/workflows/ci-toolchain-image.yml (the
// verify lines also run actionlint and yq): actions pinned by SHA, read-only
// permissions, image-build never pushes, publish is dispatch-only and its
// first step fails (never skips) when DOCKERHUB_TOKEN is empty.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const workflowPath = "../../../../.github/workflows/ci-toolchain-image.yml"

type step struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
}

type workflow struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		If    string `yaml:"if"`
		Steps []step `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadWorkflow(t *testing.T) (workflow, string) {
	t.Helper()
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	return w, string(b)
}

func TestWorkflowTriggersAndPermissions(t *testing.T) {
	w, _ := loadWorkflow(t)
	if len(w.On) != 2 || w.On["pull_request"] == nil {
		t.Errorf("triggers must be pull_request and workflow_dispatch only, got %v", w.On)
	}
	if _, ok := w.On["workflow_dispatch"]; !ok {
		t.Error("no workflow_dispatch trigger")
	}
	if len(w.Permissions) != 1 || w.Permissions["contents"] != "read" {
		t.Errorf("permissions = %v, want contents: read only", w.Permissions)
	}
}

func TestWorkflowActionsPinnedBySHA(t *testing.T) {
	w, _ := loadWorkflow(t)
	sha := regexp.MustCompile(`^[\w./-]+@[0-9a-f]{40}$`)
	n := 0
	for name, j := range w.Jobs {
		for _, s := range j.Steps {
			if s.Uses == "" {
				continue
			}
			n++
			if !sha.MatchString(s.Uses) {
				t.Errorf("job %s step %q uses %q, not pinned by commit SHA", name, s.Name, s.Uses)
			}
		}
	}
	if n == 0 {
		t.Fatal("no actions found")
	}
}

func TestWorkflowImageBuildNeverPushes(t *testing.T) {
	w, _ := loadWorkflow(t)
	j, ok := w.Jobs["image-build"]
	if !ok || j.If != "github.event_name == 'pull_request'" {
		t.Fatalf("image-build job %+v", j)
	}
	build := false
	for _, s := range j.Steps {
		if strings.HasPrefix(s.Uses, "docker/login-action") {
			t.Error("image-build logs in to a registry")
		}
		if strings.HasPrefix(s.Uses, "docker/build-push-action") {
			build = true
			if s.With["outputs"] != nil {
				t.Errorf("image-build sets outputs %v; a registry output would push", s.With["outputs"])
			}
			if s.With["push"] != false {
				t.Errorf("image-build push = %v", s.With["push"])
			}
			if s.With["platforms"] != "linux/amd64,linux/arm64" {
				t.Errorf("platforms = %v", s.With["platforms"])
			}
		}
		if strings.Contains(s.Run, "DOCKERHUB") || len(s.Env) > 0 {
			t.Errorf("image-build step %q touches secrets or env", s.Name)
		}
	}
	if !build {
		t.Error("image-build has no build step")
	}
}

func TestWorkflowPublishFailsWithoutToken(t *testing.T) {
	w, raw := loadWorkflow(t)
	j, ok := w.Jobs["publish"]
	if !ok || j.If != "github.event_name == 'workflow_dispatch'" {
		t.Fatalf("publish job %+v", j)
	}
	first := j.Steps[0]
	if first.If != "" || !strings.Contains(first.Run, "DOCKERHUB_TOKEN") || !strings.Contains(first.Run, "exit 1") || !strings.Contains(first.Run, "DOCKERHUB_TOKEN missing") {
		t.Errorf("first publish step must fail (never skip) without the token: %+v", first)
	}
	if first.Env["DOCKERHUB_TOKEN"] != "${{ secrets.DOCKERHUB_TOKEN }}" {
		t.Errorf("first step env = %v", first.Env)
	}
	mainOnly := false
	for _, st := range j.Steps[:3] {
		mainOnly = mainOnly || (strings.Contains(st.Run, "refs/heads/main") && strings.Contains(st.Run, "exit 1"))
	}
	if !mainOnly {
		t.Error("publish does not fail early when dispatched from a ref other than main")
	}
	if strings.Contains(raw, "continue-on-error") {
		t.Error("continue-on-error would let a failed publish pass")
	}
	if !strings.Contains(raw, "push-by-digest=true") {
		t.Error("publish does not push by digest")
	}
}

// TestWorkflowHelperImagesPinnedByDigest: setup-qemu and setup-buildx pull their helper images
// (privileged binfmt, buildkit) by tag unless told otherwise; both must carry a digest.
func TestWorkflowHelperImagesPinnedByDigest(t *testing.T) {
	w, _ := loadWorkflow(t)
	digest := regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	seen := map[string]int{}
	for name, j := range w.Jobs {
		for _, s := range j.Steps {
			switch {
			case strings.HasPrefix(s.Uses, "docker/setup-qemu-action"):
				seen["qemu"]++
				if img, _ := s.With["image"].(string); !strings.HasPrefix(img, "tonistiigi/binfmt:") || !digest.MatchString(img) {
					t.Errorf("job %s: setup-qemu image = %q, want tonistiigi/binfmt:<tag>@sha256:<digest>", name, img)
				}
			case strings.HasPrefix(s.Uses, "docker/setup-buildx-action"):
				seen["buildx"]++
				opts, _ := s.With["driver-opts"].(string)
				if !strings.HasPrefix(opts, "image=moby/buildkit:") || !digest.MatchString(opts) {
					t.Errorf("job %s: setup-buildx driver-opts = %q, want image=moby/buildkit:<tag>@sha256:<digest>", name, opts)
				}
			}
		}
	}
	if seen["qemu"] != 2 || seen["buildx"] != 2 {
		t.Errorf("expected both jobs to set up qemu and buildx, saw %v", seen)
	}
}

// helperImageRe matches the digest-pinned helper images the workflow hands to
// docker/setup-qemu-action (`image:`) and docker/setup-buildx-action
// (`driver-opts: image=`).
var helperImageRe = regexp.MustCompile(`(?m)\bimage[:=] ?([a-z0-9][a-z0-9._/-]*):([A-Za-z0-9._-]+)@(sha256:[0-9a-f]{64})`)

// TestWorkflowHelperImagesHavePinRows: every helper image of the workflow has a
// base-image row in the pins (same repo, tag and digest), so the weekly
// toolchain-pins drill reports a newer digest for it (D-0997).
func TestWorkflowHelperImagesHavePinRows(t *testing.T) {
	_, raw := loadWorkflow(t)
	found := helperImageRe.FindAllStringSubmatch(raw, -1)
	if len(found) < 2 {
		t.Fatalf("expected the binfmt and buildkit helper images in the workflow, found %d", len(found))
	}
	pins := loadPinsFromRepo(t)
	for _, m := range found {
		repo, tag, digest := m[1], m[2], m[3]
		ok := false
		for _, p := range pins {
			if p.Kind == KindBaseImage && strings.TrimPrefix(p.Image, "docker.io/") == repo && p.Version == tag && p.Digest == digest {
				ok = true
			}
		}
		if !ok {
			t.Errorf("workflow helper image %s:%s@%s has no matching base-image pin row", repo, tag, digest)
		}
	}
}
