package authz_test

import (
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func TestCacheAccess_Table(t *testing.T) {
	un7 := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "7")
	un8 := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "8")
	ownerMain := mustNS(t, "owner", authz.Protected, "", "")
	ownerFeat := mustNS(t, "owner", authz.Unprotected, "", "")
	internalMain := mustNS(t, "internal", authz.Protected, "", "")
	collabMain := mustNS(t, "collaborator", authz.Protected, "", "")
	other := authz.Namespace{Project: "proj-2", Trust: "owner", Protection: authz.Protected}
	rows := []struct {
		name   string
		job    authz.Namespace
		target authz.Namespace
		want   authz.Mode
	}{
		{"own", ownerMain, ownerMain, authz.ModeRWOwn},
		{"untrusted own PR", un7, un7, authz.ModeRWOwn},
		{"untrusted reads owner main snapshot", un7, ownerMain, authz.ModeROSnapshot},
		{"untrusted reads internal main snapshot", un7, internalMain, authz.ModeROSnapshot},
		{"untrusted never reads collaborator", un7, collabMain, authz.ModeNone},
		{"untrusted never reads unprotected owner", un7, ownerFeat, authz.ModeNone},
		{"untrusted PR A never touches PR B", un7, un8, authz.ModeNone},
		{"collaborator reads owner main snapshot", collabMain, ownerMain, authz.ModeROSnapshot},
		{"owner never reads lower class", ownerMain, collabMain, authz.ModeNone},
		{"owner never reads untrusted PR", ownerMain, un7, authz.ModeNone},
		{"owner feature never reads own main", ownerFeat, ownerMain, authz.ModeNone},
		{"other project", ownerMain, other, authz.ModeNone},
		{"untrusted other project", un7, other, authz.ModeNone},
		{"invalid job namespace", authz.Namespace{}, ownerMain, authz.ModeNone},
		{"invalid target", ownerMain, authz.Namespace{}, authz.ModeNone},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			if got := authz.CacheAccess(finalJob(r.job), r.target); got != r.want {
				t.Fatalf("got %s want %s", got, r.want)
			}
		})
	}
}

func TestCacheAccess_EndedJobGetsNothing(t *testing.T) {
	ns := mustNS(t, "owner", authz.Protected, "", "")
	for _, state := range []authz.AttemptState{authz.StateEnded, "", "cancelled", "FINALIZING"} {
		job := finalJob(ns)
		job.State = state
		if got := authz.CacheAccess(job, ns); got != authz.ModeNone {
			t.Fatalf("state %q: %s", state, got)
		}
	}
	for _, state := range []authz.AttemptState{authz.StateRunning, authz.StateFinalizing} {
		job := finalJob(ns)
		job.State = state
		if got := authz.CacheAccess(job, ns); got != authz.ModeRWOwn {
			t.Fatalf("state %q: %s", state, got)
		}
	}
}

func denial(t *testing.T, err error) authz.Reason {
	t.Helper()
	var d *authz.Denied
	if !errors.As(err, &d) {
		t.Fatalf("want *Denied, got %v", err)
	}
	if want := authz.CodeCacheWrite + ": cache write denied: " + string(d.Reason); len(d.Error()) < len(want) || d.Error()[:len(want)] != want {
		t.Fatalf("error text %q", d.Error())
	}
	return d.Reason
}

func TestCacheAccess_WriteCapability(t *testing.T) {
	own := mustNS(t, "internal", authz.Protected, "", "")
	if err := authz.CheckWrite(finalJob(own), own, currentCap); err != nil {
		t.Fatalf("good write refused: %v", err)
	}
	rows := []struct {
		name string
		bend func(*authz.Job, *authz.Namespace, *authz.Capability)
		want authz.Reason
	}{
		{"zombie attempt (other attempt now holds the lease)", func(j *authz.Job, _ *authz.Namespace, c *authz.Capability) { c.Attempt = "att-2" }, authz.ReasonCapability},
		{"empty attempt id", func(j *authz.Job, _ *authz.Namespace, c *authz.Capability) { j.Capability.Attempt, c.Attempt = "", "" }, authz.ReasonCapability},
		{"stale epoch", func(j *authz.Job, _ *authz.Namespace, c *authz.Capability) { c.Epoch = 4 }, authz.ReasonEpoch},
		{"replayed frame older epoch", func(j *authz.Job, _ *authz.Namespace, c *authz.Capability) { j.Capability.Epoch = 2 }, authz.ReasonEpoch},
		{"running not finalizing", func(j *authz.Job, _ *authz.Namespace, _ *authz.Capability) { j.State = authz.StateRunning }, authz.ReasonState},
		{"ended", func(j *authz.Job, _ *authz.Namespace, _ *authz.Capability) { j.State = authz.StateEnded }, authz.ReasonState},
		{"failed attempt", func(j *authz.Job, _ *authz.Namespace, _ *authz.Capability) { j.Passed = false }, authz.ReasonResult},
		{"claims another project's namespace", func(_ *authz.Job, n *authz.Namespace, _ *authz.Capability) { n.Project = "proj-2" }, authz.ReasonNamespace},
		{"claims a higher class", func(_ *authz.Job, n *authz.Namespace, _ *authz.Capability) { n.Trust = "owner" }, authz.ReasonNamespace},
		{"claims the zero namespace", func(_ *authz.Job, n *authz.Namespace, _ *authz.Capability) { *n = authz.Namespace{} }, authz.ReasonNamespace},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			job, claimed, cur := finalJob(own), own, currentCap
			r.bend(&job, &claimed, &cur)
			if got := denial(t, authz.CheckWrite(job, claimed, cur)); got != r.want {
				t.Fatalf("got %s want %s", got, r.want)
			}
		})
	}
}

// A lower class with a valid capability still cannot write a snapshot source.
func TestCacheAccess_SnapshotIsNeverWritable(t *testing.T) {
	un := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "7")
	target := mustNS(t, "owner", authz.Protected, "", "")
	if got := denial(t, authz.CheckWrite(finalJob(un), target, currentCap)); got != authz.ReasonNamespace {
		t.Fatalf("got %s", got)
	}
}
