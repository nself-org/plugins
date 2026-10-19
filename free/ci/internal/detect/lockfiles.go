package detect

import (
	"path"
	"strings"
)

var lockNames = map[string]string{
	"go.sum": "go", "package-lock.json": "npm", "npm-shrinkwrap.json": "npm",
	"pnpm-lock.yaml": "pnpm", "yarn.lock": "yarn", "bun.lock": "bun",
	"bun.lockb": "bun", "Cargo.lock": "cargo", "pubspec.lock": "pub",
	"Package.resolved": "swiftpm", "uv.lock": "uv", "poetry.lock": "poetry",
}

// lockManager names the package manager of the lockfile at p. Every
// requirements*.txt and constraints*.txt, and every .txt under a requirements/
// directory, counts: pip projects pin their inputs there (P7-CI-64, D-1029).
func lockManager(p string) (string, bool) {
	base := path.Base(p)
	if m, ok := lockNames[base]; ok {
		return m, true
	}
	if !strings.HasSuffix(base, ".txt") {
		return "", false
	}
	if strings.HasPrefix(base, "requirements") || strings.HasPrefix(base, "constraints") || path.Base(path.Dir(p)) == "requirements" {
		return "pip", true
	}
	return "", false
}

func detectLockfiles(s Snapshot) []Fact {
	var out []Fact
	for p := range s.files {
		manager, ok := lockManager(p)
		if !ok {
			continue
		}
		hash, err := s.lockDigest(p)
		if err != nil {
			out = append(out, unknown("lockfile", p, "unreadable or oversized lockfile"))
			continue
		}
		f := fact("lockfile", manager, p)
		f.SHA256 = hash
		out = append(out, f)
	}
	return out
}
