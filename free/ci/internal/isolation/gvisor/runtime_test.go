package gvisor

import "testing"

func TestRuntimeArgsAllowlist(t *testing.T) {
	for _, args := range [][]string{
		{""}, {"--strace=bogus"}, {"--future-unsafe-flag"}, {"--network=sandbox"}, {"positional"}, {"--"},
		{"--platform"}, {"--debug-log"}, {"--platform", "--strace"},
		{"--strace", "false", "--net-raw"}, {"---platform=systrap"},
	} {
		if _, denial := checkArgs(args); denial == "" {
			t.Errorf("accepted unjudgeable arguments %q", args)
		}
	}
	for _, args := range [][]string{
		nil, {"--platform=systrap"}, {"-platform", "ptrace"},
		{"--debug-log=/tmp/runsc.log", "--strace"}, {"--strace=false"},
	} {
		if _, denial := checkArgs(args); denial != "" {
			t.Errorf("refused safe arguments %q: %s", args, denial)
		}
	}
}

// Keep the historical deny list as a tripwire when the allow list changes.
func TestUnsafeRuntimeArgsTripwire(t *testing.T) {
	for _, name := range []string{"host-uds", "fsgofer-host-uds", "host-fifo", "net-raw", "allow-flag-override", "ignore-cgroups", "rootless", "nvproxy", "nvproxy-docker", "tpuproxy", "profile", "TESTONLY-example", "network", "file-access"} {
		for _, arg := range []string{"--" + name, "-" + name + "=false"} {
			if _, denial := checkArgs([]string{arg}); denial == "" {
				t.Errorf("accepted %s", arg)
			}
		}
	}
}
