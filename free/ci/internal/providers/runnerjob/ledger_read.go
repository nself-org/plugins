package runnerjob

// Purpose: the read side of the Ledger: find the registration of an attempt, list the repositories that ever
//          held a demand, list the open runners of one repository. Split out of ledger.go to keep it under 300 lines.
// Inputs:  the Ledger and its FactStore.
// Outputs: Runner, Repos, Open.
// Constraints: reads that go through load hold the Ledger mutex, like every write.

import (
	"context"
	"encoding/json"
)

// Runner returns the registration of an attempt, searching the repositories the ledger knows.
func (l *Ledger) Runner(ctx context.Context, attempt string) (Runner, bool, error) {
	l.mu.Lock()
	key, cached := l.where[attempt]
	l.mu.Unlock()
	if cached { // the common answer after a mint in this process: one document, not every repository
		if d, err := l.docOf(ctx, key.Provider, key.Repo); err == nil {
			if r, ok := d.Runners[attempt]; ok {
				return r, true, nil
			}
		}
	}
	repos, err := l.Repos(ctx)
	if err != nil {
		return Runner{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range repos {
		d, err := l.load(ctx, k.Provider, k.Repo)
		if err != nil {
			return Runner{}, false, err
		}
		if r, ok := d.Runners[attempt]; ok {
			return r, true, nil
		}
	}
	return Runner{}, false, nil
}

// docOf is load under the mutex.
func (l *Ledger) docOf(ctx context.Context, provider, repo string) (repoDoc, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.load(ctx, provider, repo)
}

// Repos lists the repositories that ever held a demand.
func (l *Ledger) Repos(ctx context.Context) ([]RepoKey, error) {
	rows, err := l.fs.ProviderFactList(ctx, factProvider, indexScope, "repo:")
	if err != nil {
		return nil, err
	}
	var out []RepoKey
	for _, f := range rows {
		var k RepoKey
		if json.Unmarshal(f.Value, &k) == nil && k.Repo != "" {
			out = append(out, k)
		}
	}
	return out, nil
}

// Open lists the runner rows of a repository that are not gone, for the sweep.
func (l *Ledger) Open(ctx context.Context, provider, repo string) ([]Runner, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, err := l.load(ctx, provider, repo)
	if err != nil {
		return nil, err
	}
	var out []Runner
	for _, r := range d.Runners {
		if r.State == stateMinted {
			out = append(out, r)
		}
	}
	return out, nil
}
