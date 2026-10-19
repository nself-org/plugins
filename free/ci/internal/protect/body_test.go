package protect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestProtectBody: the PUT body adds {nself-ci, app 15368} to the checks,
// keeps every existing check with its pin and changes nothing else.
func TestProtectBody(t *testing.T) {
	p, err := Build("example-org/example-repo", "main", fixture(t, "live_full.json"), AppID)
	if err != nil {
		t.Fatal(err)
	}
	want := fixture(t, "live_full.put.golden.json")
	if got := p.JSON(); string(got) != string(want) {
		t.Fatalf("PUT body differs from the golden\n got: %s\nwant: %s", got, want)
	}
	if len(p.Added) != 1 || p.Added[0] != (Check{Context: "nself-ci", AppID: 15368}) || p.Noop() {
		t.Fatalf("added = %+v", p.Added)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.JSON(), &doc); err != nil {
		t.Fatal(err)
	}
	rsc := doc["required_status_checks"].(map[string]any)
	if _, has := rsc["contexts"]; has {
		t.Fatal("a body must carry checks, never contexts (a contexts-only body drops every app pin)")
	}
	// Everything outside required_status_checks.checks equals the normalised live answer.
	live, err := Normalise(fixture(t, "live_full.json"))
	if err != nil {
		t.Fatal(err)
	}
	live.RequiredStatusChecks.Checks = p.Body.RequiredStatusChecks.Checks
	if d := Diff(live, p.Body); len(d) != 0 {
		t.Fatalf("fields other than the added check changed: %v", d)
	}
}

func TestProtectBodyAppIDOverride(t *testing.T) {
	p, err := Build("example-org/example-repo", "main", fixture(t, "live_full.json"), 4242)
	if err != nil {
		t.Fatal(err)
	}
	last := p.Body.RequiredStatusChecks.Checks
	if got := last[len(last)-1]; got != (Check{Context: "nself-ci", AppID: 4242}) {
		t.Fatalf("last check = %+v", got)
	}
}

func TestProtectBodyCases(t *testing.T) {
	cases := []struct {
		name, file string
		noop       bool
		wantErr    string
		checks     []Check
	}{
		{name: "unpinned check stays unpinned", file: "live_unpinned.json",
			checks: []Check{{"lint", -1}, {"build (ubuntu-latest)", 15368}, {"nself-ci", 15368}}},
		{name: "contexts only fold to unpinned checks", file: "live_contexts_only.json",
			checks: []Check{{"lint", -1}, {"build (ubuntu-latest)", -1}, {"nself-ci", 15368}}},
		{name: "already required is a no-op", file: "live_already.json", noop: true,
			checks: []Check{{"lint", 15368}, {"build (ubuntu-latest)", 15368}, {"nself-ci", 15368}}},
		{name: "another pin is a conflict", file: "live_conflict.json", wantErr: "already required with app id 9999"},
		{name: "required signatures cannot be replayed", file: "live_signatures.json", wantErr: "required_signatures"},
		{name: "unknown field refuses the plan", file: "live_unknown_key.json", wantErr: "surprise_rule"},
		{name: "no checks block gets one", file: "live_no_checks_no_reviews.json", checks: []Check{{"nself-ci", 15368}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build("example-org/example-repo", "main", fixture(t, tc.file), AppID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Noop() != tc.noop {
				t.Fatalf("noop = %t", p.Noop())
			}
			got := p.Body.RequiredStatusChecks.Checks
			if len(got) != len(tc.checks) {
				t.Fatalf("checks = %+v, want %+v", got, tc.checks)
			}
			for i := range got {
				if got[i] != tc.checks[i] {
					t.Fatalf("checks = %+v, want %+v", got, tc.checks)
				}
			}
		})
	}
}

func TestProtectBodyRestrictionsAndReviews(t *testing.T) {
	p, err := Build("example-org/example-repo", "main", fixture(t, "live_restricted.json"), AppID)
	if err != nil {
		t.Fatal(err)
	}
	r := p.Body.Restrictions
	if r == nil || strings.Join(r.Users, ",") != "amy,zed" || strings.Join(r.Teams, ",") != "core" || len(r.Apps) != 0 {
		t.Fatalf("restrictions = %+v (sorted principals expected)", r)
	}
	rv := p.Body.RequiredReviews
	if rv == nil || rv.Dismissal == nil || strings.Join(rv.Dismissal.Users, ",") != "amy" {
		t.Fatalf("reviews = %+v", rv)
	}
	if rv.Bypass != nil {
		t.Fatalf("an all-empty bypass block must be dropped, got %+v", rv.Bypass)
	}
	if !p.Body.EnforceAdmins || !p.Body.RequiredConversationRes {
		t.Fatalf("flags lost: %+v", p.Body)
	}
}

func TestProtectBodyUnprotected(t *testing.T) {
	p, err := Build("example-org/example-repo", "main", nil, AppID)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Unprotected || p.Noop() || len(p.Added) != 1 {
		t.Fatalf("plan = %+v", p)
	}
	b := p.Body
	if b.RequiredReviews != nil || b.Restrictions != nil || b.EnforceAdmins || b.AllowForcePushes {
		t.Fatalf("creating protection must set only the check: %+v", b)
	}
}

func TestNormaliseRejectsNonObject(t *testing.T) {
	if _, err := Normalise([]byte(`[]`)); err == nil {
		t.Fatal("an array is not a protection document")
	}
}

// TestNormaliseRefusesUnknownNestedKeys: a key the PUT cannot replay, inside a
// block, refuses the plan (it would otherwise be reset and read back as equal).
func TestNormaliseRefusesUnknownNestedKeys(t *testing.T) {
	base := string(fixture(t, "live_restricted.json"))
	cases := map[string]string{
		"required_pull_request_reviews.surprise":                        `"required_pull_request_reviews": {`,
		"required_status_checks.surprise":                               `"required_status_checks": {`,
		"enforce_admins.surprise":                                       `"enforce_admins": {`,
		"restrictions.surprise":                                         `"restrictions": {`,
		"required_pull_request_reviews.dismissal_restrictions.surprise": `"dismissal_restrictions": {`,
	}
	for want, anchor := range cases {
		t.Run(want, func(t *testing.T) {
			doc := strings.Replace(base, anchor, anchor+` "surprise": true,`, 1)
			if doc == base {
				t.Fatalf("anchor %q not found in the fixture", anchor)
			}
			_, err := Normalise([]byte(doc))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want it to name %s", err, want)
			}
		})
	}
	// A check item with an extra key.
	doc := strings.Replace(base, `"context": "lint",`, `"context": "lint", "surprise": 1,`, 1)
	if _, err := Normalise([]byte(doc)); err == nil || !strings.Contains(err.Error(), "checks[].surprise") {
		t.Fatalf("err = %v", err)
	}
}

// TestNormaliseAcceptsEnforcementLevel: GitHub still answers the deprecated
// enforcement_level inside required_status_checks; it is derived, not settable.
func TestNormaliseAcceptsEnforcementLevel(t *testing.T) {
	doc := strings.Replace(string(fixture(t, "live_full.json")), `"strict": false,`, `"strict": false, "enforcement_level": "non_admins",`, 1)
	if _, err := Normalise([]byte(doc)); err != nil {
		t.Fatal(err)
	}
}
