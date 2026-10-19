package gvisor

import "testing"

// D-1066: runtime arguments are judged by an allow list. A flag nobody listed is refused, so a
// runsc release that adds a sandbox-widening flag cannot slip past a deny list that predates it.

func TestCheckArgsAllowList(t *testing.T) {
	for name, tc := range map[string]struct {
		args     []string
		platform string
		denial   string
	}{
		"none":                    {nil, "systrap", ""},
		"platform equals":         {[]string{"--platform=ptrace"}, "ptrace", ""},
		"platform spaced":         {[]string{"--platform", "kvm"}, "kvm", ""},
		"single dash":             {[]string{"-platform=ptrace"}, "ptrace", ""},
		"last platform wins":      {[]string{"--platform=kvm", "--platform=systrap"}, "systrap", ""},
		"debug logging":           {[]string{"--debug", "--debug-log=/tmp/runsc/", "--log-packets"}, "systrap", ""},
		"debug explicit bool":     {[]string{"--debug=true", "--strace=false"}, "systrap", ""},
		"strace":                  {[]string{"--strace", "--strace-syscalls=open,read", "--strace-log-size=1024"}, "systrap", ""},
		"log files":               {[]string{"--log=/tmp/runsc.log", "--log-format", "json", "--panic-log=/tmp/p.log"}, "systrap", ""},
		"exclusive file access":   {[]string{"--file-access=exclusive"}, "systrap", ""},
		"sandbox network":         {[]string{"--network=sandbox"}, "systrap", ""},
		"no network":              {[]string{"--network", "none"}, "systrap", ""},
		"unlisted flag":           {[]string{"--some-future-flag"}, "", "--some-future-flag"},
		"unlisted flag with data": {[]string{"--platform=ptrace", "--some-future-flag=1"}, "", "--some-future-flag"},
		"unlisted single dash":    {[]string{"-some-future-flag"}, "", "--some-future-flag"},
		"overlay2":                {[]string{"--overlay2=all:dir=/"}, "", "--overlay2"},
		"directfs toggle":         {[]string{"--directfs=false"}, "", "--directfs"},
		"bad file access":         {[]string{"--file-access=bogus"}, "", "--file-access=bogus"},
		"bad network":             {[]string{"--network=bogus"}, "", "--network=bogus"},
		"valueless network":       {[]string{"--network"}, "", "--network="},
		"bad bool":                {[]string{"--debug=sometimes"}, "", "--debug=sometimes"},
		"stray word":              {[]string{"--debug", "orphan"}, "", "orphan"},
		"stray first word":        {[]string{"orphan"}, "", "orphan"},
		"flag eats the next flag": {[]string{"--platform", "--net-raw"}, "", "--net-raw"},
		"testonly":                {[]string{"--TESTONLY-unsafe-nonroot"}, "", "--TESTONLY-unsafe-nonroot"},
		"lone dashes":             {[]string{"--"}, "", "--"},
	} {
		t.Run(name, func(t *testing.T) {
			platform, denial := checkArgs(tc.args)
			wantDeny := tc.denial != ""
			if (denial != "") != wantDeny {
				t.Fatalf("checkArgs(%q) = (%q, %q), want denied=%v", tc.args, platform, denial, wantDeny)
			}
			if tc.denial != "" && denial != tc.denial {
				t.Errorf("denial = %q, want %q", denial, tc.denial)
			}
			if !wantDeny && platform != tc.platform {
				t.Errorf("platform = %q, want %q", platform, tc.platform)
			}
		})
	}
}

// Tripwire: the flags Epic D9 and the 2026-10-19 lead ruling name as widening the sandbox must
// stay refused, in every spelling and with any value. An allow list that grows must not take one in.
func TestWideningFlagsStayRefused(t *testing.T) {
	flags := []string{"net-raw", "host-uds", "fsgofer-host-uds", "host-fifo", "allow-flag-override",
		"ignore-cgroups", "rootless", "nvproxy", "nvproxy-docker", "tpuproxy", "profile",
		"TESTONLY-unsafe-nonroot", "testonly-allow-packet-endpoint-write"}
	for _, f := range flags {
		for _, args := range [][]string{{"--" + f}, {"-" + f}, {"--" + f + "=false"}, {"--" + f + "=open"}, {"--" + f, "all"}} {
			if _, denial := checkArgs(args); denial == "" {
				t.Errorf("checkArgs(%q) allowed a widening flag", args)
			}
		}
		if allowedFlags[f] != nil {
			t.Errorf("%s is on the allow list", f)
		}
	}
	for _, args := range [][]string{{"--network=host"}, {"--network", "host"}, {"-network=host"}, {"--file-access=shared"}, {"--file-access", "shared"}} {
		if _, denial := checkArgs(args); denial == "" {
			t.Errorf("checkArgs(%q) allowed a widening value", args)
		}
	}
}
