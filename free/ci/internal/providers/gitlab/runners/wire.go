package runners

// Purpose: the registration of the GitLab launcher: the demand handler for source gitlab and the minter for
// the gitlab-runner-token credential kind.
// Inputs:  init().
// Outputs: providers.DemandHandlerFor("gitlab") is the Handler; the Poller is a coordinator Reconciler (D-1389); runnerjob.KindOf("gitlab") is
//          gitlab-runner-token; RegistrationErrors lists anything init could not register.
// Constraints: nothing runs unless compat.V15(). The ledger is the launcher's one shared ledger: the
//          coordinator start binds it once (github/demand.Bind, which calls runnerjob.Use), so there is no
//          second Bind here; until it is bound the handler answers E704 and the demand is offered again.
//          The coordinator start also installs UsePolicy of this package next to runtime.UsePolicy
//          (P7-NODE-20 wiring). Without an installed policy the minter uses the defaults, which name no
//          runner_token_env, so it answers E700 and never falls back to the Developer token.

import (
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

func init() {
	if err := runnerjob.InstallCoordinator(); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
	if err := providers.RegisterDemandHandler(Source, &Handler{}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
	if err := runnerjob.RegisterMinter(CredentialKind, &Minter{}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
	if err := coordinator.RegisterReconciler(&Poller{}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}
