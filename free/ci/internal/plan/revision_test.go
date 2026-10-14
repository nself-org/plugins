package plan

import "testing"

func TestClassifyRevision(t *testing.T) {
	for _, c := range []struct {
		name         string
		f            BranchFacts
		def, release bool
		defName      string
	}{
		{"detached head", BranchFacts{Detached: true, Remotes: []string{"main"}}, true, false, "main"},
		{"master repo, no origin/HEAD, on master", BranchFacts{Branch: "master", Remotes: []string{"master"}}, true, false, "master"},
		{"master repo, no origin/HEAD, feature", BranchFacts{Branch: "feature/x", Remotes: []string{"master"}}, false, false, "master"},
		{"origin/HEAD set, on default", BranchFacts{Branch: "trunk", OriginHead: "trunk"}, true, false, "trunk"},
		{"origin/HEAD set, feature", BranchFacts{Branch: "feature/x", OriginHead: "main"}, false, false, "main"},
		{"no origin at all", BranchFacts{Branch: "anything"}, true, false, ""},
		{"empty branch", BranchFacts{}, true, false, ""},
		{"tag checkout", BranchFacts{Detached: true, ExactTag: true}, true, true, ""},
		{"release on feature branch", BranchFacts{Branch: "f", OriginHead: "main", ExactTag: true}, false, true, "main"},
	} {
		d, r := ClassifyRevision(c.f)
		if d != c.def || r != c.release || DefaultBranchName(c.f) != c.defName {
			t.Errorf("%s: default=%v release=%v name=%q", c.name, d, r, DefaultBranchName(c.f))
		}
	}
}
