package hygiene

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestProbeUsernsDecidesFromWhatTheNamespaceCouldRead(t *testing.T) {
	real := usernsProbe
	t.Cleanup(func() { usernsProbe = real })
	cases := []struct {
		name     string
		readable bool
		ok       bool
		err      error
		want     Probe
	}{
		{"denied is separation", false, true, nil, Probe{Available: true, Reason: ProbeOK}},
		{"readable is not separation", true, true, nil, Probe{Reason: ProbeReadable}},
		{"unshare could not set up", false, false, nil, Probe{Reason: ProbeUnavail}},
		{"unshare missing", false, false, errors.New("exec: not found"), Probe{Reason: ProbeUnavail}},
	}
	for _, c := range cases {
		usernsProbe = func(_ context.Context, sub int, file string) (bool, bool, error) {
			if sub != 300000 {
				t.Errorf("%s: sub uid %d", c.name, sub)
			}
			if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
				t.Errorf("%s: probe file %v %v", c.name, info, err)
			}
			return c.readable, c.ok, c.err
		}
		got, err := ProbeUserns(context.Background(), t.TempDir(), 300000)
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v", c.name, got, err)
		}
	}
	if got, _ := ProbeUserns(context.Background(), t.TempDir(), 0); got.Reason != ProbeNoSubuid || got.Available {
		t.Errorf("no subordinate range: %+v", got)
	}
}
