package run

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// persistentGoCacheDir is the persistent Go cache directory of namespace ns: the first 24 hex of the
// sha256 of its canonical, length-prefixed encoding (so no two namespaces can collide).
func persistentGoCacheDir(home string, ns authz.Namespace) string {
	h := sha256.Sum256(ns.Canonical())
	return filepath.Join(home, "gocache", hex.EncodeToString(h[:])[:24])
}

// setupPersistentGoCache creates the persistent Go cache directories and
// returns the environment variables pointing to them. It creates dir/build
// and dir/mod with 0700 permissions.
func setupPersistentGoCache(home string, ns authz.Namespace) (CacheSetup, error) {
	var none CacheSetup
	dir := persistentGoCacheDir(home, ns)
	buildDir := filepath.Join(dir, "build")
	modDir := filepath.Join(dir, "mod")
	if err := os.MkdirAll(buildDir, 0700); err != nil {
		return none, fmt.Errorf("create GOCACHE dir: %w", err)
	}
	if err := os.MkdirAll(modDir, 0700); err != nil {
		return none, fmt.Errorf("create GOMODCACHE dir: %w", err)
	}
	return CacheSetup{Env: map[string]string{
		"GOCACHE":    buildDir,
		"GOMODCACHE": modDir,
	}}, nil
}

// hasGoEcosystem returns true if the detected facts include the Go ecosystem.
func hasGoEcosystem(detected []detect.Fact) bool {
	for _, sp := range specs.For(detected, false) { // the same applicability the snapshot path uses
		if sp.ID == "go" {
			return true
		}
	}
	return false
}

// tryPersistentGoCache attempts to set up a persistent Go cache for process
// jobs with Go ecosystem. It returns the setup and true if successful, or
// an empty setup and false if the job should use the snapshot path instead.
func tryPersistentGoCache(home string, container bool, detected []detect.Fact, facts entries.Facts) (CacheSetup, bool) {
	if container {
		return CacheSetup{}, false
	}
	if !hasGoEcosystem(detected) {
		return CacheSetup{}, false
	}
	ns, err := entries.NamespaceOf(facts) // project, trust class, protection, head repo and PR: different trust never shares a dir
	if err != nil {
		return CacheSetup{}, false
	}
	setup, err := setupPersistentGoCache(home, ns)
	if err != nil {
		return CacheSetup{}, false
	}
	return setup, true
}
