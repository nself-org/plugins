//go:build fault

package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestFaultKillMidTransition(t *testing.T) {
	if os.Getenv("CI31_FAULT_CHILD") == "1" {
		path := os.Getenv("CI31_DB")
		ready := os.Getenv("CI31_READY")
		s, err := Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		tx, err := s.writer.Begin()
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec("UPDATE attempt SET state='running',epoch=epoch+1 WHERE id='a'")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(ready, []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		_ = tx.Rollback()
		return
	}
	s := testStore(t)
	seed(t, s, "a")
	path := s.path
	for i := 0; i < 100; i++ {
		ready := filepath.Join(t.TempDir(), "ready")
		cmd := exec.Command(os.Args[0], "-test.run=^TestFaultKillMidTransition$")
		cmd.Env = append(os.Environ(), "CI31_FAULT_CHILD=1", "CI31_DB="+path, "CI31_READY="+ready)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = cmd.Process.Kill()
				t.Fatal("child did not reach transaction")
			}
			time.Sleep(time.Millisecond)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		var state string
		var epoch int64
		if err := s.readers.QueryRow("SELECT state,epoch FROM attempt WHERE id='a'").Scan(&state, &epoch); err != nil || state != "queued" || epoch != 0 {
			t.Fatalf("partial commit iteration %d: %s %d %v", i, state, epoch, err)
		}
	}
}

func TestTwoProcessAdvance(t *testing.T) {
	if os.Getenv("CI31_ADMIT_CHILD") == "1" {
		s, err := Open(os.Getenv("CI31_DB"), Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		id := os.Getenv("CI31_ADMIT_ID")
		err = s.AdmitAndLease(context.Background(), "shared", Demand{AttemptID: id, RunnerID: "runner", LeaseID: "lease" + id, TokenDigest: "digest", CPU: 1, MemMB: 1, CapacityCPU: 1, CapacityMemMB: 1, Epoch: 0})
		if err != nil && !codeIs(err, "E604") {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("CI31_ADVANCE_CHILD") == "1" {
		s, err := Open(os.Getenv("CI31_DB"), Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		part, _ := strconv.Atoi(os.Getenv("CI31_PART"))
		for i := part; i < 1000; i += 2 {
			id := fmt.Sprintf("a%04d", i)
			if err := s.AdmitAndLease(context.Background(), "bulk", Demand{AttemptID: id, RunnerID: "runner", LeaseID: "lease" + id, TokenDigest: "digest", CPU: 1, MemMB: 1, CapacityCPU: 1000, CapacityMemMB: 1000, Epoch: 0}); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	s := testStore(t)
	ctx := context.Background()
	p := PipelineRow{ID: "p", Revision: "rev", Project: "p", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := s.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("a%04d", i)
		if err := s.CreateJob(ctx, JobRow{ID: "j" + id, PipelineID: "p", Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
	}
	cmds := []*exec.Cmd{}
	outputs := []*bytes.Buffer{}
	for part := 0; part < 2; part++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTwoProcessAdvance$")
		cmd.Env = append(os.Environ(), "CI31_ADVANCE_CHILD=1", "CI31_DB="+s.path, fmt.Sprintf("CI31_PART=%d", part))
		output := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
		outputs = append(outputs, output)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child %d: %v: %s", i, err, outputs[i].String())
		}
	}
	var n int
	if err := s.readers.QueryRow("SELECT count(*) FROM attempt WHERE state='leased' AND epoch=1").Scan(&n); err != nil || n != 1000 {
		t.Fatalf("advanced %d attempts: %v", n, err)
	}
	if err := s.readers.QueryRow("SELECT count(*) FROM lease WHERE host='bulk'").Scan(&n); err != nil || n != 1000 {
		t.Fatalf("bulk leases %d: %v", n, err)
	}
	for _, id := range []string{"host-a", "host-b"} {
		if err := s.CreateJob(ctx, JobRow{ID: "j" + id, PipelineID: "p", Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
	}
	cmds = nil
	outputs = nil
	for _, id := range []string{"host-a", "host-b"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTwoProcessAdvance$")
		cmd.Env = append(os.Environ(), "CI31_ADMIT_CHILD=1", "CI31_DB="+s.path, "CI31_ADMIT_ID="+id)
		output := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
		outputs = append(outputs, output)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("host child %d: %v: %s", i, err, outputs[i].String())
		}
	}
	if err := s.readers.QueryRow("SELECT count(*) FROM host_reservation WHERE host='shared'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("host reservation %d: %v", n, err)
	}
}

func TestVersionSkewOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path, Options{ReaderVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.writer.Exec("UPDATE schema_meta SET schema_version=2,min_reader_version=1"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path, Options{ReaderVersion: 1})
	if err != nil {
		t.Fatal("compatible expansion:", err)
	}
	if _, err = s.writer.Exec("UPDATE schema_meta SET min_reader_version=2"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = Open(path, Options{ReaderVersion: 1})
	if !codeIs(err, "E606") {
		t.Fatalf("expected E606: %v", err)
	}
}
