package schedpolicy

// Purpose: the per-run flags `--prefer`, `--runner` and `--max-queue` as
// revision-scope policy values (Epic D10; X17, R-DX-06: a foreground run
// waits at most 10 minutes for a runner unless told otherwise).
// Inputs: the raw flag strings and whether the run is in the foreground.
// Outputs: a flat key to raw JSON map, the shape trust.Scope.Values takes.
// Constraints: pure; every value passes the key's own decoder, so an unknown
// preset or a bad duration is E691 before a run starts; trust's tighten rules
// decide what the merge keeps (the revision scope can only tighten).

import (
	"encoding/json"
	"slices"
	"time"
)

// ForegroundMaxQueue is the queue wait of a foreground run without --max-queue.
const ForegroundMaxQueue = 10 * time.Minute

// Flags are the raw per-run scheduling flags; empty means not given.
type Flags struct {
	Prefer     string // --prefer: a preset name, or its short form ("local" for "prefer-local")
	Runner     string // --runner
	MaxQueue   string // --max-queue: a Go duration
	Foreground bool   // a person at a terminal waits; a queued run does not
}

// PresetFromFlag maps a --prefer value to a preset name: the name itself, or
// the name with its "prefer-" prefix left off.
func PresetFromFlag(v string) (string, error) {
	names := PresetNames()
	if slices.Contains(names, v) {
		return v, nil
	}
	if slices.Contains(names, "prefer-"+v) {
		return "prefer-" + v, nil
	}
	return "", fail(KeyPreset, "unknown preset %q (one of %v)", v, names)
}

// CheckRunner refuses an empty --runner value.
func CheckRunner(v string) error {
	if v == "" {
		return fail(KeyRunner, "expected a non-empty runner id")
	}
	return nil
}

// Validate checks every given flag value without building the map.
func (f Flags) Validate() error {
	_, err := f.Values()
	return err
}

// Values returns the revision-scope values the flags stand for. A foreground
// run without --max-queue gets ForegroundMaxQueue.
func (f Flags) Values() (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	put := func(key string, v any) {
		raw, _ := json.Marshal(v) // a string always marshals
		out[key] = raw
	}
	if f.Prefer != "" {
		name, err := PresetFromFlag(f.Prefer)
		if err != nil {
			return nil, err
		}
		put(KeyPreset, name)
	}
	if f.Runner != "" {
		put(KeyRunner, f.Runner)
	}
	queue := f.MaxQueue
	if queue == "" && f.Foreground {
		queue = ForegroundMaxQueue.String()
	}
	if queue != "" {
		if _, err := ParseDurationMs(KeyMaxQueue, queue); err != nil {
			return nil, err
		}
		put(KeyMaxQueue, queue)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
