package main

import "testing"

func TestServedRefOption(t *testing.T) {
	saved := optionsHooks
	optionsHooks = nil
	defer func() { optionsHooks = saved }()
	t.Setenv("NSELF_CI_REF", "refs/heads/served")
	o, err := parseBare(nil)
	if err != nil || o.Ref != "refs/heads/served" {
		t.Fatalf("ref %q error %v", o.Ref, err)
	}
}
