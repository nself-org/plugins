package authz

// Purpose: who may see an artifact record and bytes (Epic CACHE D6, ADR 0025
// §11 roles). The caller maps "not visible" and "not found" to one code (E713)
// so existence never leaks.
// Inputs:  the artifact's project and visibility, the viewer's project and role.
// Outputs: ArtifactVisibility.
// Constraints: pure; default deny. Roles are ordered member < release < admin;
// an unknown role, project or visibility is the most restrictive reading.

// Role is a TEAM role (P7-TRUST-12 owns the grants; only the order is here).
type Role string

const (
	RoleMember  Role = "member"
	RoleRelease Role = "release"
	RoleAdmin   Role = "admin"
)

// Visibility is the artifact's declared audience.
type Visibility string

const (
	// VisibilityProject (the default) is every member of the project.
	VisibilityProject Visibility = "project"
	// VisibilityRestricted is release and admin roles only.
	VisibilityRestricted Visibility = "restricted"
)

func roleRank(r Role) int {
	switch r {
	case RoleMember:
		return 1
	case RoleRelease:
		return 2
	case RoleAdmin:
		return 3
	}
	return 0
}

// ArtifactMeta is the stored record's authorization-relevant part.
type ArtifactMeta struct {
	Project    string
	Visibility Visibility // empty means the default, project
}

// Viewer is the authenticated caller. Project is the project its token or
// grant is scoped to; Role is the role held in it.
type Viewer struct {
	Project string
	Role    Role
}

// ArtifactVisibility reports whether v may see a. A viewer of another project
// never sees it, whatever the role; an unknown visibility value reads as
// restricted.
func ArtifactVisibility(a ArtifactMeta, v Viewer) bool {
	if a.Project == "" || a.Project != v.Project {
		return false
	}
	need := roleRank(RoleRelease)
	if a.Visibility == "" || a.Visibility == VisibilityProject {
		need = roleRank(RoleMember)
	}
	return roleRank(v.Role) >= need
}
