package main

// Purpose: interpret one batched remote-ref listing with the same exact-name
// semantics as three individual rev-parse probes.
// Inputs: for-each-ref output, its success flag and the legacy query fallback.
// Outputs: existing main, master and trunk names, in their original order.
// Constraints: prefix matches never count; a failed listing retains the former
// per-ref behavior instead of guessing that the repository has no refs.

import "strings"

func planRemoteNames(out string, ok bool, git func(...string) (string, bool)) []string {
	listed := "\n" + out + "\n"
	var names []string
	for _, name := range []string{"main", "master", "trunk"} {
		found := strings.Contains(listed, "\nrefs/remotes/origin/"+name+"\n")
		if !ok {
			_, found = git("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+name)
		}
		if found {
			names = append(names, name)
		}
	}
	return names
}
