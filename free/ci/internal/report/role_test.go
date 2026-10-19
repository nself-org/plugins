package report

import "testing"

// TestRoleDefault proves acceptance: the default role is RolePersonal.
func TestRoleDefault(t *testing.T) {
	SetProcessRole(RolePersonal)
	if got := ProcessRole(); got != RolePersonal {
		t.Fatalf("default role = %v, want personal", got)
	}
	if RolePersonal != 0 {
		t.Fatalf("RolePersonal must be the zero value, got %d", RolePersonal)
	}
}

// TestRoleSetGet proves acceptance: SetProcessRole then ProcessRole round-trips.
func TestRoleSetGet(t *testing.T) {
	t.Cleanup(func() { SetProcessRole(RolePersonal) })
	for _, r := range []Role{RoleTeam, RolePersonal, RoleTeam} {
		SetProcessRole(r)
		if got := ProcessRole(); got != r {
			t.Fatalf("ProcessRole() = %v after SetProcessRole(%v)", got, r)
		}
	}
	if RoleTeam.String() != "team" || RolePersonal.String() != "personal" {
		t.Fatal("role names changed")
	}
}
