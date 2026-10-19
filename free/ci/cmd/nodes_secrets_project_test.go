package main

// Purpose: a release or deploy grant goes only to a dedicated node whose single authorized project is
// the project of the file being granted (D-1071); a node dedicated to project A never gets project B's file.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// dedicatedCapability is an operator-owned node that accepts exactly owner and has one authorized project.
func dedicatedCapability(projects ...string) model.Capability {
	var c model.Capability
	op := "operator"
	c.Identity.Ownership = model.Fact[string]{Value: &op, Source: "assigned"}
	accepts := []model.TrustClass{"owner"}
	c.Trust.Accepts = model.Fact[[]model.TrustClass]{Value: &accepts, Source: "assigned"}
	c.Lifecycle.AuthorizedProjects = projects
	return c
}

func TestDedicatedGrantNeedsMatchingProject(t *testing.T) {
	cases := []struct {
		name     string
		c        model.Capability
		class    model.SecretClass
		project  string
		wantCode bool
	}{
		{"same project, release", dedicatedCapability("/work/a"), "release", "/work/a", false},
		{"same project, deploy", dedicatedCapability("/work/a"), "deploy", "/work/a", false},
		{"other project, release", dedicatedCapability("/work/a"), "release", "/work/b", true},
		{"other project, deploy", dedicatedCapability("/work/a"), "deploy", "/work/b", true},
		{"project prefix is not a match", dedicatedCapability("/work/a"), "release", "/work/a/sub", true},
		{"project class is not bound", dedicatedCapability("/work/a"), "project", "/work/b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := dedicatedGrantRefusal("node-1", tc.class, tc.c, tc.project)
			if (err != nil) != tc.wantCode {
				t.Fatalf("refusal = %v, want refusal %v", err, tc.wantCode)
			}
			if err != nil && (err.Code != "E677" || !strings.Contains(err.Message, "node-1") || len(err.Reasons) != 1 || err.Reasons[0] != secrets.ReasonNodeOtherProject) {
				t.Fatalf("refusal = %+v", err)
			}
		})
	}
}

func TestDedicatedGrantRefusesNonDedicatedFirst(t *testing.T) {
	c := dedicatedCapability("/work/a", "/work/b") // two projects: not dedicated
	err := dedicatedGrantRefusal("node-1", "release", c, "/work/a")
	if err == nil || err.Code != "E677" || err.Reasons[0] == secrets.ReasonNodeOtherProject {
		t.Fatalf("refusal = %+v", err)
	}
}
