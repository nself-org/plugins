package runtime

// Purpose: let a provider say what a valid repository path (the nself.repo node label) looks like for
//          it, so the one AttemptFacts source of the driver does not assume GitHub's owner/name (D-1345).
// Inputs:  RegisterRepoCheck from a provider's init(); RepoCheck from a FactsSource.
// Outputs: the check function of a provider id, or nil when the provider registered none.
// Constraints: a provider with no check keeps the owner/name rule of the FactsSource's own provider;
//          a repeated registration replaces the earlier one (init order is not defined).

import "sync"

var repoChecks struct {
	sync.RWMutex
	m map[string]func(string) bool
}

// RegisterRepoCheck installs the repository path check of a provider id (the "nself.provider=" label).
func RegisterRepoCheck(provider string, check func(string) bool) {
	repoChecks.Lock()
	defer repoChecks.Unlock()
	if repoChecks.m == nil {
		repoChecks.m = map[string]func(string) bool{}
	}
	repoChecks.m[provider] = check
}

// RepoCheck returns the repository path check of a provider id, or nil.
func RepoCheck(provider string) func(string) bool {
	repoChecks.RLock()
	defer repoChecks.RUnlock()
	return repoChecks.m[provider]
}
