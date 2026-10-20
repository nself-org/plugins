package demand_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

func codeOf(err error) (code, reason string, class providers.ErrorClass) {
	if pe, ok := err.(*providers.Error); ok {
		return pe.Code, pe.Reason, pe.Class
	}
	return "", "", ""
}

// TestLabelFilter proves acceptance line 1 (review F5): a queued workflow_job with labels [self-hosted,
// nself] becomes one Synthetic provider-runner pipeline in the background band; any label outside the fixed
// set, a missing self-hosted or jit label, or a macOS or Windows label is E708 label.not_allowed and no
// pipeline; a completed or in_progress delivery never reaches the handler (the webhook source declines).
func TestLabelFilter(t *testing.T) {
	bind(t)
	h := handler(50)
	allowed := [][]string{
		{"self-hosted", "nself"}, {"self-hosted", "nself", "linux", "x64"}, {"self-hosted", "nself", "linux", "arm64"},
		{"Self-Hosted", "NSELF", "Linux", "X64"}, {"nself", "self-hosted", "linux"},
	}
	for i, labels := range allowed {
		p, err := h.Synthesize(context.Background(), demandFor(t, 5000+i, labels...))
		if err != nil || p == nil {
			t.Fatalf("%v: want a pipeline, got %v", labels, err)
		}
		j := p.Jobs[0]
		wantArch := "linux/amd64"
		if strings.EqualFold(labels[len(labels)-1], "arm64") {
			wantArch = "linux/arm64"
		}
		if p.Name != "provider-runner" || p.Kind != "adhoc" || runnerjob.BandOf(p) != "background" || j.Matrix.Platform != wantArch ||
			j.Container.Image != testImage || j.RunnerCredential != "github-jitconfig" || !j.RemoteOnly || j.Isolation != "sandboxed-container" || len(j.Secrets) != 0 {
			t.Fatalf("%v: pipeline %+v job %+v", labels, p, j)
		}
		if j.Env[runnerjob.EnvDemand] != itoa(5000+i) || j.Env[runnerjob.EnvRepo] != testRepo || j.Env[runnerjob.EnvProvider] != "github" {
			t.Fatalf("%v: env %v", labels, j.Env)
		}
	}
	refused := [][]string{
		{"self-hosted", "nself", "ubuntu-latest"}, {"self-hosted", "nself", "ubuntu-22.04"}, {"self-hosted", "nself", "macOS"},
		{"self-hosted", "nself", "windows-latest"}, {"self-hosted", "nself", "gpu"}, {"self-hosted", "nself", "nself-jit-01ABC"},
		{"self-hosted"}, {"nself"}, {"self-hosted", "linux", "x64"}, {"ubuntu-latest"},
		{"self-hosted", "nself", "l\u0130nux"}, {"self-hosted", "nself", "linux\u200b"}, {"self-hosted", "nself\u2010x"}, {"self-hosted", "", "nself"},
	}
	for i, labels := range refused {
		p, err := h.Synthesize(context.Background(), demandFor(t, 6000+i, labels...))
		code, reason, class := codeOf(err)
		if p != nil || code != "E708" || reason != "label.not_allowed" || class != providers.Config {
			t.Errorf("%v: want E708 label.not_allowed as a refusal, got %v %q %q pipeline %v", labels, code, reason, class, p)
		}
	}
	for _, action := range []string{"completed", "in_progress", "waiting"} {
		if _, err := verified(t, jobPayload(action, 7000, "self-hosted", "nself")); err == nil {
			t.Errorf("a %s workflow_job must not become a demand", action)
		}
	}
}

// TestLabelFilterFollowsTheOperatorLabel: the allowed set uses the operator's jit label, not the word nself.
func TestLabelFilterFollowsTheOperatorLabel(t *testing.T) {
	bind(t)
	h := handler(5)
	h.Settings = func(context.Context) (demandSettings, error) {
		return demandSettings{Label: "ci-box", MaxPending: 5}, nil
	}
	if p, err := h.Synthesize(context.Background(), demandFor(t, 1, "self-hosted", "ci-box")); err != nil || p == nil {
		t.Fatalf("operator label: %v", err)
	}
	if _, err := h.Synthesize(context.Background(), demandFor(t, 2, "self-hosted", "nself")); err == nil {
		t.Fatal("the default label is not allowed when the operator chose another")
	}
}

// TestDemandCap proves acceptance line 2 (review F8): a 20-job matrix of [self-hosted, nself] demands
// creates 2 live pipelines and 18 answers runner.demand_capped (a Quota class error, so the poll offers
// the job again); the same job re-offered while live keeps its slot and takes no second one; after one
// finishes the next demand is accepted; another repository has its own slots.
func TestDemandCap(t *testing.T) {
	bind(t)
	h := handler(2)
	ctx := context.Background()
	ok, capped := 0, 0
	for i := 1; i <= 20; i++ {
		p, err := h.Synthesize(ctx, demandFor(t, 8000+i, "self-hosted", "nself"))
		switch code, reason, class := codeOf(err); {
		case err == nil && p != nil:
			ok++
		case code == "E703" && reason == "runner.demand_capped" && class == providers.Quota:
			capped++
		default:
			t.Fatalf("job %d: %v", i, err)
		}
	}
	if ok != 2 || capped != 18 {
		t.Fatalf("accepted %d capped %d", ok, capped)
	}
	if p, err := h.Synthesize(ctx, demandFor(t, 8001, "self-hosted", "nself")); err != nil || p == nil {
		t.Fatalf("a live demand offered again must be accepted without a second slot: %v", err)
	}
	led := runnerjob.CurrentLedger()
	if err := led.RecordRunner(ctx, runnerjob.Runner{Attempt: "a1", Provider: "github", Repo: testRepo, Demand: "8001", Handle: "repo:1"}); err != nil {
		t.Fatal(err)
	}
	row, _, _ := led.Runner(ctx, "a1")
	if err := led.MarkGone(ctx, row); err != nil {
		t.Fatal(err)
	}
	if p, err := h.Synthesize(ctx, demandFor(t, 8003, "self-hosted", "nself")); err != nil || p == nil {
		t.Fatalf("after one finished the next demand is accepted: %v", err)
	}
	if _, err := h.Synthesize(ctx, demandFor(t, 8004, "self-hosted", "nself")); err == nil {
		t.Fatal("still capped at two")
	}
}

// TestHandlerFailsClosed: unbound ledger and v1.4 mode and malformed facts all answer without a pipeline,
// and only the first two are retried (not refusals).
func TestHandlerFailsClosed(t *testing.T) {
	bind(t)
	h := handler(2)
	f := demandFor(t, 9001, "self-hosted", "nself")
	runnerjob.Use(nil)
	if p, err := h.Synthesize(context.Background(), f); p != nil || func() providers.ErrorClass { _, _, c := codeOf(err); return c }() != providers.Transient {
		t.Fatalf("unbound ledger must be a retry: %v", err)
	}
	bind(t)
	t.Setenv("NSELF_V15", "0")
	if p, err := h.Synthesize(context.Background(), f); p != nil || err == nil {
		t.Fatalf("v1.4 mode must refuse: %v", err)
	}
	t.Setenv("NSELF_V15", "1")
	bad := f
	bad.ProviderJobID = model.Fact[string]{}
	if p, err := h.Synthesize(context.Background(), bad); p != nil || err == nil {
		t.Fatalf("no job id: %v", err)
	}
	bad = f
	bad.Kind = "push"
	if p, err := h.Synthesize(context.Background(), bad); p != nil || err == nil {
		t.Fatalf("not a demand: %v", err)
	}
	unpinned := handler(2)
	unpinned.Image = func() (string, error) { return "ghcr.io/actions/actions-runner:2.338.0", nil }
	if p, err := unpinned.Synthesize(context.Background(), f); p != nil || err == nil {
		t.Fatalf("an image by tag must be refused: %v", err)
	}
}
