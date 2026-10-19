package authz_test

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func TestArtifactVisibility_Table(t *testing.T) {
	proj := func(v authz.Visibility) authz.ArtifactMeta { return authz.ArtifactMeta{Project: "p1", Visibility: v} }
	viewer := func(role authz.Role) authz.Viewer { return authz.Viewer{Project: "p1", Role: role} }
	rows := []struct {
		name string
		a    authz.ArtifactMeta
		v    authz.Viewer
		want bool
	}{
		{"member sees project artifact", proj(authz.VisibilityProject), viewer(authz.RoleMember), true},
		{"default visibility is project", proj(""), viewer(authz.RoleMember), true},
		{"member cannot see restricted", proj(authz.VisibilityRestricted), viewer(authz.RoleMember), false},
		{"release sees restricted", proj(authz.VisibilityRestricted), viewer(authz.RoleRelease), true},
		{"admin sees restricted", proj(authz.VisibilityRestricted), viewer(authz.RoleAdmin), true},
		{"unknown visibility reads restricted", proj("secret"), viewer(authz.RoleMember), false},
		{"unknown role sees nothing", proj(authz.VisibilityProject), viewer("root"), false},
		{"no role sees nothing", proj(authz.VisibilityProject), viewer(""), false},
		{"other project admin", proj(authz.VisibilityProject), authz.Viewer{Project: "p2", Role: authz.RoleAdmin}, false},
		{"viewer without project", proj(authz.VisibilityProject), authz.Viewer{Role: authz.RoleAdmin}, false},
		{"artifact without project", authz.ArtifactMeta{}, authz.Viewer{Role: authz.RoleAdmin}, false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			if got := authz.ArtifactVisibility(r.a, r.v); got != r.want {
				t.Fatalf("got %v want %v", got, r.want)
			}
		})
	}
}
