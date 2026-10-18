package authz_test

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func goodEntry(t *testing.T) authz.EntryFacts {
	return authz.EntryFacts{Namespace: mustNS(t, "owner", authz.Protected, "", ""),
		NoSecretsReceived: true, NoPrivateDeps: true, ScannedClean: true}
}

var cleanProject = authz.ProjectFacts{NoPrivateDeps: true}

func TestSnapshotEligible_Table(t *testing.T) {
	rows := []struct {
		name string
		bend func(*authz.EntryFacts, *authz.ProjectFacts)
		want bool
	}{
		{"zero-secret protected trusted entry", func(*authz.EntryFacts, *authz.ProjectFacts) {}, true},
		{"internal class too", func(e *authz.EntryFacts, _ *authz.ProjectFacts) {
			e.Namespace = mustNS(t, "internal", authz.Protected, "", "")
		}, true},
		{"writer received a secret", func(e *authz.EntryFacts, _ *authz.ProjectFacts) { e.NoSecretsReceived = false }, false},
		{"entry has private deps", func(e *authz.EntryFacts, _ *authz.ProjectFacts) { e.NoPrivateDeps = false }, false},
		{"project flagged private deps", func(_ *authz.EntryFacts, p *authz.ProjectFacts) { p.NoPrivateDeps = false }, false},
		{"not scanned clean", func(e *authz.EntryFacts, _ *authz.ProjectFacts) { e.ScannedClean = false }, false},
		{"unprotected namespace", func(e *authz.EntryFacts, _ *authz.ProjectFacts) {
			e.Namespace = mustNS(t, "owner", authz.Unprotected, "", "")
		}, false},
		{"collaborator namespace", func(e *authz.EntryFacts, _ *authz.ProjectFacts) {
			e.Namespace = mustNS(t, "collaborator", authz.Protected, "", "")
		}, false},
		{"untrusted namespace", func(e *authz.EntryFacts, _ *authz.ProjectFacts) {
			e.Namespace = mustNS(t, "untrusted", authz.Unprotected, "f/r", "1")
		}, false},
		{"zero namespace", func(e *authz.EntryFacts, _ *authz.ProjectFacts) { e.Namespace = authz.Namespace{} }, false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			e, p := goodEntry(t), cleanProject
			r.bend(&e, &p)
			if got := authz.SnapshotEligible(e, p); got != r.want {
				t.Fatalf("got %v want %v", got, r.want)
			}
		})
	}
}

// Unknown facts are not eligible: the zero value of every fact says "not attested".
func TestSnapshotEligible_ZeroValueIsNotEligible(t *testing.T) {
	if authz.SnapshotEligible(authz.EntryFacts{}, authz.ProjectFacts{}) {
		t.Fatal("zero facts eligible")
	}
	e := goodEntry(t)
	if authz.SnapshotEligible(e, authz.ProjectFacts{}) {
		t.Fatal("zero project facts eligible")
	}
}
