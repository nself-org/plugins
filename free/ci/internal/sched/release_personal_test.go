package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// personalReleaseWorld is the operator's own machine under a PERSONAL coordinator (no VM, no UID
// separation) and a release-secret job of the operator's own run (P7-CACHE-15, R-DX-12).
func personalReleaseWorld() (Runner, Job) {
	r := fixtureRunner("local", "local")
	r.Decl.Accepts = []model.TrustClass{"owner", "internal"}
	r.Decl.Isolation, r.Decl.UIDSeparation, r.Decl.Projects = "process", false, 0
	r.Decl.SecretClasses = []model.SecretClass{"release", "project"}
	r.CoordinatorMode, r.CoordinatorHost = trust.Personal, true
	j := fixtureJob("android")
	j.SecretClasses = []model.SecretClass{"release"}
	return r, j
}

func refusedWith(t *testing.T, r Runner, j Job, code string) {
	t.Helper()
	got := Place(fixtureWorld(r), []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, code) {
		t.Fatalf("placed or refused for another reason (want %s): %+v", code, got)
	}
}

// The operator's own node runs the operator's own release-secret job: it is the dedicated trusted
// case, so neither release_needs_dedicated nor the VM isolation minimum applies.
func TestPersonalOwnNodePlacesOwnReleaseJob(t *testing.T) {
	r, j := personalReleaseWorld()
	if got := Place(fixtureWorld(r), []Job{j}); len(got.Placements) != 1 {
		t.Fatalf("the operator's own release job was not placed: %+v", got.Explanations)
	}
}

// Nothing else becomes dedicated: another node, another run, a shared or wider-trust node, a weaker
// job trust and a job without release secrets keep the rules they had.
func TestReleaseSecretJobsStayRefusedElsewhere(t *testing.T) {
	cases := map[string]func(*Runner, *Job){
		"not the operator's own node": func(r *Runner, _ *Job) { r.Decl.PersonalOwnNode = false },
		"not the operator's own run":  func(_ *Runner, j *Job) { j.PersonalOwnRun = false },
		"shared node":                 func(r *Runner, _ *Job) { r.Decl.Shared = true },
		"node accepts collaborators":  func(r *Runner, _ *Job) { r.Decl.Accepts = append(r.Decl.Accepts, "collaborator") },
		"collaborator job":            func(_ *Runner, j *Job) { j.Trust = "collaborator" },
		"hello-declared node":         func(r *Runner, _ *Job) { r.Decl.Source = "hello" },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			r, j := personalReleaseWorld()
			edit(&r, &j)
			refusedWith(t, r, j, string(trust.TrustReleaseNeedsDedicated))
		})
	}
	t.Run("a project-secret job is not judged as a release job", func(t *testing.T) {
		r, j := personalReleaseWorld()
		j.SecretClasses = []model.SecretClass{"project"}
		r.Decl.PersonalOwnNode = false
		got := Place(fixtureWorld(r), []Job{j})
		if len(got.Explanations) > 0 && len(got.Explanations[0].Alternatives) > 0 && containsCode(got.Explanations[0].Alternatives[0].Reasons, string(trust.TrustReleaseNeedsDedicated)) {
			t.Fatalf("release rule applied to project secrets: %+v", got.Explanations)
		}
	})
}
