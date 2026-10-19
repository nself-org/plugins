//go:build integration

package ssh

// Purpose: the hostile-operator-config and secret-canary scenarios of TestSSHTransport.
// Inputs:  the rig of integration_rig_test.go.
// Constraints: the operator config is applied through an ssh shim that adds `-F`, which is
// how a ~/.ssh/config reaches ssh; the D4 options must still win.

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (r *itRig) hostileConfig(t *testing.T) {
	dir := t.TempDir()
	marker, ctl := filepath.Join(dir, "local-command-ran"), filepath.Join(dir, "ctl")
	if err := os.Mkdir(ctl, 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fwd := ln.Addr().String()
	_ = ln.Close() // a port ssh would have to open itself
	conf := fmt.Sprintf(`Host *
  ForwardAgent yes
  ForwardX11 yes
  LocalForward %s 127.0.0.1:22
  PermitLocalCommand yes
  LocalCommand touch %s
  ControlMaster auto
  ControlPath %s/%%C
  ControlPersist 60
  RemoteCommand touch /tmp/remote-command-ran
  VerifyHostKeyDNS yes
  StrictHostKeyChecking no
  SendEnv SECRET_*
`, fwd, marker, ctl)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	real, err := exec.LookPath("ssh")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	shim := fmt.Sprintf("#!/bin/sh\nexec %s -F %s \"$@\"\n", real, filepath.Join(dir, "config"))
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "agent.sock"))
	t.Setenv("DISPLAY", ":99")
	t.Setenv("SECRET_TOKEN", "hostile-env-canary")
	r.sh("n1", "printf 'echo LOGIN-BANNER\\n' | cat - /home/nself/.bashrc > /tmp/bashrc.new && mv /tmp/bashrc.new /home/nself/.bashrc && chown nself:nself /home/nself/.bashrc")

	var env, dial string
	var once sync.Once
	probed := make(chan struct{})
	sink := newLineSink(func(s string) {
		if s != "ready" {
			return
		}
		once.Do(func() {
			go func() {
				defer close(probed)
				env = r.sh("n1", `for p in $(pgrep -f '[n]self-ci-agent serve'); do tr '\0' '\n' < /proc/$p/environ; done`)
				if c, err := net.DialTimeout("tcp", fwd, 500*time.Millisecond); err == nil {
					_ = c.Close()
					dial = "open"
				}
			}()
		})
	})
	if err := r.drv.Run(context.Background(), r.node("n1"), r.lease("it-hostile", "sh", "-c", "echo ready; sleep 2"), sink); err != nil {
		t.Fatal(err)
	}
	<-probed
	if !strings.Contains(env, "LANG=C") {
		t.Fatalf("the probe did not read the agent's environment: %q", env)
	}
	for _, bad := range []string{"SSH_AUTH_SOCK", "DISPLAY", "SECRET_", "hostile-env-canary"} {
		if strings.Contains(env, bad) {
			t.Errorf("the node's agent sees %s", bad)
		}
	}
	if dial != "" {
		t.Error("a local forward was opened")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("LocalCommand ran")
	}
	if entries, _ := os.ReadDir(ctl); len(entries) != 0 {
		t.Errorf("a control socket was created: %v", entries)
	}
	if r.sh("n1", "test -e /tmp/remote-command-ran && echo ran || echo no") != "no" {
		t.Error("the configured RemoteCommand ran instead of the agent")
	}
}

func (r *itRig) secretCanary(t *testing.T) {
	const canary = "CANARY-s3cr3t-7f3a91c2d4e5"
	d := r.driver()
	d.Personal = true
	d.Secrets = func(context.Context, protocol.Lease) ([]protocol.SecretValue, error) {
		return []protocol.SecretValue{{Ref: "API_TOKEN", Value: canary}}, nil
	}
	l := r.lease("it-secret", "sh", "-c", `echo "token=$API_TOKEN"; echo ready; sleep 2`)
	l.Job.Secrets = []string{"API_TOKEN"}
	l.Secrets = []protocol.SecretRef{{Ref: "API_TOKEN", Class: "project"}}
	var host, nodeArgs string
	var once sync.Once
	probed := make(chan struct{})
	sink := newLineSink(func(s string) {
		if s != "ready" {
			return
		}
		once.Do(func() {
			go func() {
				defer close(probed)
				host = hostSSHView(t)
				nodeArgs = r.sh("n1", "ps -eo args")
			}()
		})
	})
	if err := d.Run(context.Background(), r.node("n1"), l, sink); err != nil {
		t.Fatal(err)
	}
	<-probed
	if !strings.Contains(host, "nself-ci-n1") || !strings.Contains(nodeArgs, "nself-ci-agent") {
		t.Fatalf("the scans saw nothing (host %d bytes, node %d bytes)", len(host), len(nodeArgs))
	}
	r.sh("n1", "mkdir -p /tmp/ctl && echo "+canary+" > /tmp/ctl/seen")
	if r.sh("n1", "grep -rlF "+canary+" /tmp/ctl || echo none") == "none" {
		t.Fatal("positive control: the grep over the node does not find a planted canary")
	}
	r.sh("n1", "rm -rf /tmp/ctl")
	for name, hay := range map[string]string{"ssh argv or environment": host, "node process list": nodeArgs, "logs": strings.Join(sink.lines(), "\n")} {
		if strings.Contains(hay, canary) {
			t.Errorf("secret value found in %s", name)
		}
	}
	if got := r.sh("n1", "grep -rlF "+canary+" /home/nself /tmp /var/tmp 2>/dev/null || echo none"); got != "none" {
		t.Errorf("secret value left on the node's disk: %s", got)
	}
}

// hostSSHView returns argv and, where the OS allows, the environment of the ssh clients of node n1.
func hostSSHView(t *testing.T) string {
	t.Helper()
	pids, _ := exec.Command("pgrep", "-f", "HostKeyAlias=nself-ci-n1").Output()
	var out strings.Builder
	for _, pid := range strings.Fields(string(pids)) {
		a, _ := exec.Command("ps", "-ww", "-p", pid, "-o", "command=").Output()
		out.Write(a)
		if b, err := os.ReadFile("/proc/" + pid + "/environ"); err == nil {
			out.WriteString(strings.ReplaceAll(string(b), "\x00", "\n"))
		} else if e, err := exec.Command("ps", "eww", "-p", pid).Output(); err == nil {
			out.Write(e)
		}
	}
	return out.String()
}
