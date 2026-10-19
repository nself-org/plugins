package schedule

// Purpose: the operator-scope policy key triggers.schedules (Epic X1, D10): the list of
// {repo, pipeline, cron} entries the coordinator fires, and its typed view.
// Inputs:  *policy.Effective from the P7-TRUST-27 resolver (UsePolicy installs the source).
// Outputs: []Entry; the key registers from init().
// Constraints: operator scope only. The key carries Spend: true and TightenFixed, which is how the
//          policy engine keeps it out of the protected branch, the revision, the pipeline and a run
//          (E671): a repository must never choose when it is built. Kind Fixed needs a nil registered
//          default (E670 otherwise), so "no schedules" is the absence of the key. Decode is strict: an
//          unknown field, an invalid cron, a bad repo or pipeline name, a duplicate entry or more than
//          maxEntries entries is E672, the whole value is refused.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// KeySchedules is the policy key.
const KeySchedules = "triggers.schedules"

const (
	keyOwner   = "P7-HOST-20"
	maxEntries = 256
	// maxDeliveryID is the intake's cap on a delivery id (coordinator/intake.go).
	maxDeliveryID = 256
)

// Entry is one scheduled pipeline.
type Entry struct {
	Repo     string `json:"repo"`
	Pipeline string `json:"pipeline"`
	Cron     string `json:"cron"`
	spec     Spec
}

// Spec returns the parsed cron expression.
func (e Entry) Spec() (Spec, error) {
	if e.spec.expr != "" {
		return e.spec, nil
	}
	return Parse(e.Cron)
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when the key is in the
// process-wide key set.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

// Keys returns the spec of the key.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{{Key: KeySchedules, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: Decode}}
}

// RegisterKeys adds the key to the process-wide set and returns what failed.
func RegisterKeys() []error {
	var errs []error
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func init() { registrationErrors = RegisterKeys() }

func fail(format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", KeySchedules, fmt.Sprintf(format, a...))
}

// Decode is the strict decoder of the key: a JSON list of {repo, pipeline, cron} objects. It returns
// []Entry with every cron parsed.
func Decode(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var list []Entry
	if err := d.Decode(&list); err != nil {
		return nil, fail("expected a list of {repo, pipeline, cron} objects with no other field")
	}
	if list == nil {
		return nil, fail("expected a list")
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return nil, fail("expected exactly one list and nothing after it")
	}
	if len(list) > maxEntries {
		return nil, fail("at most %d entries", maxEntries)
	}
	seen := map[string]bool{}
	for i := range list {
		e := &list[i]
		if !coordinator.ValidProject(e.Repo) {
			return nil, fail("entry %d: repo must be owner/repo", i)
		}
		if e.Pipeline != "" && !coordinator.ValidPipeline(e.Pipeline) {
			return nil, fail("entry %d: pipeline must be a plain name or empty", i)
		}
		if len(DeliveryID(e.Repo, e.Pipeline, time.Time{})) > maxDeliveryID {
			return nil, fail("entry %d: repo and pipeline are too long for a delivery id", i)
		}
		spec, err := Parse(e.Cron)
		if err != nil {
			return nil, fail("entry %d: %v", i, err)
		}
		e.spec = spec
		id := markKey(*e)
		if seen[id] {
			return nil, fail("entry %d repeats repo, pipeline and cron", i)
		}
		seen[id] = true
	}
	return list, nil
}

// EntriesFrom reads the effective policy. A nil Effective is an error: silently firing nothing would
// drop the operator's schedules. No value set is no schedules.
func EntriesFrom(eff *policy.Effective) ([]Entry, error) {
	if eff == nil {
		return nil, fmt.Errorf("schedule: no effective policy")
	}
	v, _, ok := eff.Value(KeySchedules)
	if !ok {
		return nil, nil
	}
	list, isList := v.([]Entry)
	if !isList {
		return nil, fail("effective value is not a schedule list")
	}
	return list, nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this
// once). With none installed there are no schedules. Passing nil removes it.
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	policySource.Lock()
	defer policySource.Unlock()
	policySource.fn = fn
}

// CurrentEntries resolves the schedules through the installed policy source.
func CurrentEntries(ctx context.Context) ([]Entry, error) {
	policySource.RLock()
	fn := policySource.fn
	policySource.RUnlock()
	if fn == nil {
		return nil, nil
	}
	eff, err := fn(ctx)
	if err != nil {
		return nil, err
	}
	return EntriesFrom(eff)
}
