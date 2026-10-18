//go:build !windows

package exec

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRuntimeLabelledRefusesUnsafeArguments(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"list: other binary", func() error { _, err := RuntimeListLabelled(ctx, "sh", KindContainer, "a1"); return err }},
		{"list: unknown kind", func() error { _, err := RuntimeListLabelled(ctx, "docker", "image", "a1"); return err }},
		{"list: option-shaped attempt", func() error { _, err := RuntimeListLabelled(ctx, "docker", KindContainer, "--all"); return err }},
		{"list: label injection", func() error { _, err := RuntimeListLabelled(ctx, "docker", KindVolume, "a1,label=x"); return err }},
		{"remove: other binary", func() error { return RuntimeRemoveLabelled(ctx, "rm", KindContainer, "abc", "a1") }},
		{"remove: unknown kind", func() error { return RuntimeRemoveLabelled(ctx, "docker", "system", "abc", "a1") }},
		{"remove: option-shaped id", func() error { return RuntimeRemoveLabelled(ctx, "docker", KindContainer, "-f", "a1") }},
		{"remove: option-shaped attempt", func() error { return RuntimeRemoveLabelled(ctx, "docker", KindVolume, "abc", "--all") }},
		{"remove: path id", func() error { return RuntimeRemoveLabelled(ctx, "docker", KindNetwork, "a/b", "a1") }},
	} {
		var e *Error
		if err := tc.call(); err == nil || !asError(err, &e) || e.Code != "E613" {
			t.Errorf("%s: want E613, got %v", tc.name, err)
		}
	}
}

func TestKillAttemptGroupKillsOnlyAMatchingGroup(t *testing.T) {
	home := t.TempDir()
	if err := KillAttemptGroup(home, "no-marker"); err != nil {
		t.Fatalf("no marker must be a no-op: %v", err)
	}
	if err := KillAttemptGroup(home, "../x"); err == nil {
		t.Fatal("unsafe attempt accepted")
	}
	start := func() *osexec.Cmd {
		cmd := osexec.Command("sleep", "60")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	reaped := func(cmd *osexec.Cmd) bool {
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
			return true
		case <-time.After(2 * time.Second):
			return false
		}
	}
	// matching identity: the group dies and the marker goes
	victim := start()
	if err := writeProcessMarker(home, "a1", "coord", victim.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := KillAttemptGroup(home, "a1"); err != nil {
		t.Fatal(err)
	}
	if !reaped(victim) {
		t.Fatal("the marked group survived")
	}
	if _, err := os.Stat(markerPath(home, "a1")); !os.IsNotExist(err) {
		t.Fatal("marker survived")
	}
	// a reused PID (identity differs): the process is left alone
	bystander := start()
	data, _ := json.Marshal(processMarker{PID: bystander.Process.Pid, Coordinator: "coord", Identity: "Thu Jan  1 00:00:00 1970"})
	if err := os.MkdirAll(home+"/processes", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath(home, "a2"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := KillAttemptGroup(home, "a2"); err != nil {
		t.Fatal(err)
	}
	if syscall.Kill(bystander.Process.Pid, 0) != nil {
		t.Fatal("a process with a different identity was killed")
	}
	// a corrupt marker is dropped, not trusted
	if err := os.WriteFile(markerPath(home, "a3"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := KillAttemptGroup(home, "a3"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath(home, "a3")); !os.IsNotExist(err) {
		t.Fatal("corrupt marker survived")
	}
}

func TestUsernsProbeRefusesBadInput(t *testing.T) {
	for _, tc := range []struct {
		sub  int
		file string
	}{{0, "/etc/hostname"}, {-1, "/etc/hostname"}, {200000, "relative/file"}, {200000, "/etc/host\nname"}} {
		if _, _, err := UsernsProbe(context.Background(), tc.sub, tc.file); err == nil {
			t.Errorf("accepted %+v", tc)
		}
	}
}
