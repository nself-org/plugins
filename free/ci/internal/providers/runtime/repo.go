package runtime

// Purpose: the repository-path check of each hosted provider, so the one AttemptFacts source
// accepts the shape of the provider the node belongs to (D-1345): GitHub names a repository
// "owner/name", GitLab names a project by a path of two to twenty segments.
// Inputs:  the provider id (node.Identity.Provider) and the nself.repo label value.
// Outputs: whether the value is a well-formed repository path for that provider.
// Constraints: a provider with no registered check is held to the GitHub shape by the caller (the
//          facts source), so an unknown provider never widens what is accepted; a check only
//          judges the text, it calls nothing.

import (
	"sync"
)

var repoChecks = struct {
	sync.RWMutex
	m map[string]func(string) bool
}{m: map[string]func(string) bool{}}

// RegisterRepoCheck installs the repository-path check of provider; a later call replaces it.
func RegisterRepoCheck(provider string, ok func(repo string) bool) {
	repoChecks.Lock()
	defer repoChecks.Unlock()
	if ok == nil {
		delete(repoChecks.m, provider)
		return
	}
	repoChecks.m[provider] = ok
}

// RepoCheck returns the check registered for provider.
func RepoCheck(provider string) (func(repo string) bool, bool) {
	repoChecks.RLock()
	defer repoChecks.RUnlock()
	ok, found := repoChecks.m[provider]
	return ok, found
}
