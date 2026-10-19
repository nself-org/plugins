package job

// Purpose: the environment rules of a release job (Epic D9): it receives
// NSELF_CI_IDEMPOTENCY_KEY, and CI never receives deploy credentials.
// Inputs:  the attempt id, a job definition, an environment list.
// Outputs: the key and a refusal of a job that declares a deploy variable.
// Constraints: the key is the one run.CreateAttempt derives for at-most-once
// jobs (internal/run/release_hook.go checks the two stay equal); a name is a
// deploy variable when it starts with DEPLOY_ or NSELF_DEPLOY_, any case. The
// operator's own environment reaches a job only through exec.AllowlistEnv,
// which carries no such name; CheckDeclared runs on the final, merged job
// environment (release_hook.go wraps the isolator for that).

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// KeyEnv is the variable that carries the idempotency key into the job.
const KeyEnv = "NSELF_CI_IDEMPOTENCY_KEY"

// IdempotencyKey is the key of an at-most-once attempt: hex sha256 of a
// domain-separated attempt id.
func IdempotencyKey(attemptID string) string {
	sum := sha256.Sum256([]byte("nself-ci-idempotency\x00" + attemptID))
	return hex.EncodeToString(sum[:])
}

// IsDeployVar reports whether name is a deploy credential variable.
func IsDeployVar(name string) bool {
	n := strings.ToUpper(name)
	return strings.HasPrefix(n, "DEPLOY_") || strings.HasPrefix(n, "NSELF_DEPLOY_")
}

// CheckDeclared refuses a release job that declares a deploy variable in its
// env or asks for a deploy secret.
func CheckDeclared(j model.Job) *Error {
	var bad []string
	for k := range j.Env {
		if IsDeployVar(k) {
			bad = append(bad, k)
		}
	}
	for _, s := range j.Secrets {
		if IsDeployVar(s) {
			bad = append(bad, s)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return refuse(CodeNotAdmitted, DeployEnv, "a release job must not receive deploy credentials (%s)", strings.Join(bad, ", "))
}
