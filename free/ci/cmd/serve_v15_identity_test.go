package main

import "testing"

func TestSubmitterIdentity(t *testing.T) {
	for _, c := range []struct{ submitter, project, user, team string }{
		{"user:Bob role=member", "acme/web", "bob", "acme"},
		{"token:tok-1:mia role=admin", "acme/web", "mia", "acme"},
		{"operator:cli:ali role=owner", "o/r", "ali", "o"},
		{"", "acme/web", "", "acme"},
		{"user:bob role=member", "noslash", "bob", ""},
	} {
		if u, tm := submitterIdentity(c.submitter, c.project); u != c.user || tm != c.team {
			t.Errorf("submitterIdentity(%q, %q) = %q, %q; want %q, %q", c.submitter, c.project, u, tm, c.user, c.team)
		}
	}
}
