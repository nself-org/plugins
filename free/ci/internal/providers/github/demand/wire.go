package demand

// Purpose: the registration of the GitHub launcher: the demand handler for source github, the minter
// for the github-jitconfig credential kind, and Bind, which the coordinator start calls with its store.
// Inputs:  init(); Bind(store) once at coordinator start, before the first demand.
// Outputs: providers.DemandHandlerFor("github") is the Handler; runnerjob.KindOf("github") is
//          github-jitconfig; RegistrationErrors lists anything init could not register.
// Constraints: the handler answers E704 (retry) until Bind has installed the ledger, so a demand that
//          arrives early is offered again and never lost; nothing runs unless compat.V15().

import (
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
}

// Bind gives the launcher its durable ledger (the coordinator's *store.Store implements FactStore).
func Bind(fs runnerjob.FactStore) { runnerjob.Use(runnerjob.NewLedger(fs, nil)) }
