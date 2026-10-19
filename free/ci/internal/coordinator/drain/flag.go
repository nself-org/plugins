package drain

// Purpose: the --drain-deadline serve flag (registered through coordinator.RegisterFlag) and the SIGTERM
//          channel the composition root feeds into Drainer.Serve.
// Inputs:  the serve flag set; the process's SIGTERM.
// Outputs: FlagDeadline (the parsed value or DefaultDeadline) and NotifyTerm.
// Constraints: the flag value is stored atomically so parsing several flag sets (help, tests) never races;
//          a non-positive or unparsable duration is a flag error; NotifyTerm's goroutine is awaited by stop.

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

var flagDeadline atomic.Int64

func init() {
	coordinator.RegisterFlag(func(fs *flag.FlagSet) {
		fs.Func("drain-deadline", "How long running jobs may finish after SIGTERM before they are cancelled and handed back (default 10m)", func(v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}
			if d <= 0 {
				return fmt.Errorf("drain deadline %q must be positive", v)
			}
			flagDeadline.Store(int64(d))
			return nil
		})
	})
}

// FlagDeadline is the value of --drain-deadline, or DefaultDeadline when the flag was not given.
func FlagDeadline() time.Duration {
	if d := time.Duration(flagDeadline.Load()); d > 0 {
		return d
	}
	return DefaultDeadline
}

// NotifyTerm returns a channel that closes on the first SIGTERM. stop releases the signal handler and
// waits for the goroutine that watches it.
func NotifyTerm() (term <-chan struct{}, stop func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	out := make(chan struct{})
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-sigs:
			close(out)
		case <-quit:
		}
	}()
	var once sync.Once
	return out, func() {
		once.Do(func() {
			signal.Stop(sigs)
			close(quit)
			wg.Wait()
		})
	}
}
