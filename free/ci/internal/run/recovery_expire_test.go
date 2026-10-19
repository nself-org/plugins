//go:build !windows

package run

// D-1008: coordinator rows are never removed, so a recovery that expired every
// dead coordinator again wrote one transaction per past run on every later run.

import (
	"context"
	"os/exec"
	"strconv"
	"testing"
)

func TestRecoverExpiresADeadCoordinatorOnce(t *testing.T) {
	st, home := recoveryStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		cmd := exec.Command("true")
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Register(ctx, cmd.Process.Pid, strconv.Itoa(1000+i), bootID()); err != nil {
			t.Fatal(err)
		}
	}
	first, join := RecoverBeside(ctx, st, "", home)
	_ = join()
	if first.Expired != 3 {
		t.Fatalf("first recovery expired %d dead coordinators, want 3 (errors %v)", first.Expired, first.Errors)
	}
	second, join := RecoverBeside(ctx, st, "", home)
	_ = join()
	if second.Expired != 0 {
		t.Fatalf("second recovery expired %d coordinators again, want 0", second.Expired)
	}
	all, err := st.Coordinators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all {
		if c.HeartbeatAt != 0 {
			t.Fatalf("coordinator %s was not expired", c.ID)
		}
	}
}
