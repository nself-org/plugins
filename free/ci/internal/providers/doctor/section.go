package doctor

// Purpose: registers the `providers` section with the doctor registry (P7-CI-50) under --providers
// and runs the checks of this package in id order.
// Inputs:  doctor.Env; the CI store when one exists.
// Outputs: the checks CI-DP001 to CI-DP014.
// Constraints: registration happens from init() (CI D16); the section is offline; the store is opened
// read-only in effect (never created) and closed before the section returns; a repository whose
// policy cannot be resolved still gets every check that does not need it.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// SectionName is the registered section.
const SectionName = "providers"

func init() {
	doctor.RegisterSection(doctor.Section{Name: SectionName, Selector: doctor.SelectProviders, Run: Run})
}

type state struct {
	env doctor.Env
	cfg Config
	st  *store.Store
}

// Run returns the checks of the providers section.
func Run(ctx context.Context, env doctor.Env) []doctor.Check {
	st, err := OpenExisting(env.Getenv)
	if err != nil {
		st = nil
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	return RunWith(ctx, env, st)
}

// RunWith is Run with the store given (nil: none). Tests use it.
func RunWith(ctx context.Context, env doctor.Env, st *store.Store) []doctor.Check {
	s := &state{env: env, st: st}
	s.cfg = Load(ctx, env.Root, policy.Runner(env.Capture), st)
	var out []doctor.Check
	src, kind := s.checkGitHubCredential(ctx)
	out = append(out, src, kind)
	glTok, glRun := s.checkGitLabCredentials()
	out = append(out, glTok, glRun, s.checkPolicy())
	def, unt := s.checkWorkflows(ctx)
	out = append(out, def, unt)
	out = append(out, s.checkWebhookSecrets()...)
	out = append(out, s.checkForkApproval(ctx))
	out = append(out, s.checkHealth(ctx)...)
	out = append(out, s.checkForgejo(), s.checkRegistration(), s.checkAuthorization())
	return out
}
