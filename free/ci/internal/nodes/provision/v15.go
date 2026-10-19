package provision

// Purpose: the V15 behaviour of `nodes provision` and `nodes verify` (P7-NODE-06): one v1 envelope per
// --json run through sdk/go/output, the pinned runner and the registry record of a provisioned runner.
// Inputs:  the parsed command state from cli.go and the V15 seams of Deps (RunnerPin, Record).
// Outputs: `{"schema_version":"1","command":"ci nodes provision|verify","data":...}` on stdout, or an
// error envelope with an E-code and the exit class; exit 0, 1 (user), 2 (infra).
// Constraints: nothing here runs in v1.4 mode (callers gate on compat.V15()); stdout holds exactly the
// one envelope; messages are one cut line, with the registration token already redacted by the
// provision step; verify keeps the v1.4 exit rule (1 when a check fails, a host is unreachable or
// hosts drift) and still prints its data envelope.

import (
	"context"
	"io"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

// Codes of the envelope errors. E669 is NODE-06's own; the others are the fabric's existing codes.
const (
	codeUsage = "E651"
	codeInfra = "E607"
)

// ProvisionData is the data member of the provision envelope.
type ProvisionData struct {
	Host      string     `json:"host"`
	Instances int        `json:"instances"`
	Steps     []stepData `json:"steps"`
	// Node is the registry id of the recorded runner host ("" when nothing was recorded).
	Node string `json:"node,omitempty"`
}

// VerifyData is the data member of the verify envelope.
type VerifyData struct {
	Hosts []HostReport   `json:"hosts"`
	Drift []DriftFinding `json:"drift"`
}

// stepData is the JSON form of a ProvisionStep.
type stepData struct {
	Name   string `json:"name"`
	Output string `json:"output"`
}

// failEnvelope writes the error envelope of command and returns the exit status.
func failEnvelope(w io.Writer, command string, err error) int {
	code, class := codeInfra, output.ExitInfra
	switch reason := bootstrap.ReasonOf(err); {
	case reason == bootstrap.ReasonUsage:
		code, class = bootstrap.CodeBootstrap, output.ExitUser
	case reason != "":
		code = bootstrap.CodeBootstrap
	case isUsage(err):
		code, class = codeUsage, output.ExitUser
	}
	_ = output.WriteError(w, command, output.ErrorDetail{Code: code, Message: bootstrap.Detail(err), ExitCode: class}, nil)
	return class
}

// usageError marks an argument error (missing flag, too many hosts).
type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

func isUsage(err error) bool {
	_, ok := err.(usageError)
	return ok
}

// writeVerifyEnvelope prints the verify envelope and returns the verify exit status.
func writeVerifyEnvelope(w io.Writer, reports []HostReport) int {
	data := VerifyData{Hosts: reports, Drift: DetectDrift(reports)}
	if data.Hosts == nil {
		data.Hosts = []HostReport{}
	}
	if data.Drift == nil {
		data.Drift = []DriftFinding{}
	}
	if err := output.WriteData(w, "ci nodes verify", data, nil); err != nil {
		return output.ExitInfra
	}
	if FoundProblems(reports) {
		return output.ExitUser
	}
	return 0
}

// recordRunner records the provisioned host in the node registry when the composition root wired one.
func recordRunner(ctx context.Context, d Deps, host string, labels []string) (string, error) {
	if d.Record == nil || host == "local" || host == "" {
		return "", nil
	}
	return d.Record(ctx, RecordRequest{Host: host, Labels: labels})
}
