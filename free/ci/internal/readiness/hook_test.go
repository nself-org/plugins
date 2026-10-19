package readiness

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

func runnerOf(res sample.Result, err error, seen *[]string, timeout *time.Duration) sample.Runner {
	return sample.RunnerFunc(func(_ context.Context, argv []string, d time.Duration) (sample.Result, error) {
		if seen != nil {
			*seen = argv
		}
		if timeout != nil {
			*timeout = d
		}
		return res, err
	})
}

func TestReadinessCmdExitCodes(t *testing.T) {
	var argv []string
	var timeout time.Duration
	cases := []struct {
		name   string
		res    sample.Result
		err    error
		want   HookResult
		inWhy  string
		notice bool // refuses the host
	}{
		{"exit 0 is ready", sample.Result{}, nil, HookReady, "", false},
		{"exit 3 is not ready with its reason", sample.Result{ExitCode: 3, Stdout: "\n  on a video call  \nsecond line\n"}, nil, HookNotReady, "on a video call", true},
		{"exit 3 without text still says why", sample.Result{ExitCode: 3}, nil, HookNotReady, "reported not ready", true},
		{"exit 1 is unknown", sample.Result{ExitCode: 1, Stdout: "ready!"}, nil, HookUnknown, "E800 readiness.cmd exited 1", true},
		{"exit 2 is unknown", sample.Result{ExitCode: 2}, nil, HookUnknown, "exited 2", true},
		{"timeout is unknown", sample.Result{}, errors.New("timed out after 5s"), HookUnknown, "E800 readiness.cmd did not finish: timed out after 5s", true},
		{"start failure is unknown", sample.Result{}, errors.New("fork/exec /nope: no such file"), HookUnknown, "no such file", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RunHook(context.Background(), runnerOf(tc.res, tc.err, &argv, &timeout), []string{"/usr/local/bin/busy", "--quiet"})
			if !h.Ran || h.Result != tc.want || !strings.Contains(h.Why, tc.inWhy) {
				t.Fatalf("hook %+v", h)
			}
			if strings.Contains(h.Why, "\n") {
				t.Fatalf("reason must be one line: %q", h.Why)
			}
			r := Decide(Config{}, Inputs{Hook: h})
			if r.Ready == tc.notice {
				t.Fatalf("ready=%v for %+v", r.Ready, h)
			}
			if timeout != HookTimeout || len(argv) != 2 || argv[0] != "/usr/local/bin/busy" {
				t.Fatalf("argv %v timeout %v", argv, timeout)
			}
		})
	}
	if h := RunHook(context.Background(), runnerOf(sample.Result{}, nil, nil, nil), nil); h.Ran {
		t.Fatal("no cmd configured must not run a hook")
	}
}

func TestHookReasonIsClippedAndRedacted(t *testing.T) {
	long := strings.Repeat("busy ", 100)
	h := RunHook(context.Background(), runnerOf(sample.Result{ExitCode: 3, Stdout: long}, nil, nil, nil), []string{"x"})
	if n := len([]rune(h.Why)); n > MaxWhyLen {
		t.Fatalf("reason has %d characters", n)
	}
	// A token-shaped value never reaches a reason (the shape is assembled so no literal secret sits in source).
	token := "gh" + "p_" + strings.Repeat("A1b2", 9)
	h = RunHook(context.Background(), runnerOf(sample.Result{ExitCode: 3, Stdout: "token " + token + "\x1b[31m red"}, nil, nil, nil), []string{"x"})
	if strings.Contains(h.Why, token) || strings.ContainsRune(h.Why, 0x1b) {
		t.Fatalf("reason leaked or kept a control character: %q", h.Why)
	}
}

func TestYieldTo(t *testing.T) {
	running := []string{"launchd", "zoom.us", "Safari", "obs", "a-very-long-process-name"}
	got := MatchYield([]string{"obs", "zoom.us", "zoom.us", "nothing", " ", "a-very-long-process-name-extended"}, running)
	// The long configured name matches the 15-character prefix a Linux comm would show.
	if len(got) != 2 || got[0] != "obs" || got[1] != "zoom.us" {
		t.Fatalf("matches %v", got)
	}
	if got := MatchYield([]string{"a-very-long-process-name"}, []string{"a-very-long-pro"}); len(got) != 1 {
		t.Fatalf("truncated comm must match: %v", got)
	}
	if got := MatchYield([]string{"obs"}, []string{"OBS", "obs-helper"}); len(got) != 0 {
		t.Fatalf("matching is exact: %v", got)
	}

	ev := Evaluator{
		Config:  func(context.Context) (Config, error) { return Config{YieldTo: []string{"zoom.us", "obs"}}, nil },
		Sampler: sample.Sampler{OS: "darwin", Cores: 4, Run: psOnly("launchd\n/Applications/zoom.us.app/Contents/MacOS/zoom.us\nSafari\n"), FreeBytes: func(string) (uint64, error) { return 1 << 40, nil }},
	}
	r, err := ev.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Ready || len(r.YieldedTo) != 1 || r.YieldedTo[0] != "zoom.us" || !strings.Contains(strings.Join(r.Why, "|"), "yielding to zoom.us") {
		t.Fatalf("report %+v", r)
	}
	// A host that cannot list processes cannot honour yield_to.
	ev.Sampler.Run = sample.RunnerFunc(func(context.Context, []string, time.Duration) (sample.Result, error) {
		return sample.Result{}, errors.New("ps missing")
	})
	if r, _ = ev.Evaluate(context.Background()); r.Ready || !strings.Contains(strings.Join(r.Why, "|"), "yield_to could not be checked") {
		t.Fatalf("report %+v", r)
	}
}

// psOnly answers `ps` and fails the macOS probes, whose values the test switches off or ignores.
func psOnly(ps string) sample.Runner {
	return sample.RunnerFunc(func(_ context.Context, argv []string, _ time.Duration) (sample.Result, error) {
		if argv[0] == "/bin/ps" {
			return sample.Result{Stdout: ps}, nil
		}
		return sample.Result{}, errors.New("not available in this test")
	})
}
