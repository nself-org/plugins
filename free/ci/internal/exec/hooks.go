package exec

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// EnvPolicy constructs the complete environment for a job.
type EnvPolicy interface {
	Environment(context.Context, model.Job) ([]string, error)
}

// Redactor transforms each output chunk before it reaches any sink.
// Implementations retain state when a secret spans chunks.
type Redactor interface {
	Redact([]byte) []byte
	Flush() []byte
}

// SecretSource resolves declared references for a job.
type SecretSource interface {
	Resolve(context.Context, []string) (map[string]string, error)
}

// Isolator may amend a local run or reject an unsupported isolation level.
type Isolator interface {
	Prepare(context.Context, *JobSpec) error
}

type inheritEnvironment struct{}

func (inheritEnvironment) Environment(_ context.Context, _ model.Job) ([]string, error) {
	return os.Environ(), nil
}

type passThroughRedactor struct{}

func (passThroughRedactor) Redact(p []byte) []byte { return p }
func (passThroughRedactor) Flush() []byte          { return nil }

type emptySecrets struct{}

func (emptySecrets) Resolve(_ context.Context, refs []string) (map[string]string, error) {
	if len(refs) != 0 {
		return nil, coded("E612", "secret source is required")
	}
	return nil, nil
}

type baselineIsolator struct{}

func (baselineIsolator) Prepare(_ context.Context, s *JobSpec) error {
	if s.Job.Isolation != "" && s.Job.Isolation != "process" && s.Job.Isolation != "container" {
		return coded("E611", "unsupported isolation")
	}
	return nil
}
