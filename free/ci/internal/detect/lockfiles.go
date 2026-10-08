package detect

var lockNames = map[string]string{
	"go.sum": "go", "package-lock.json": "npm", "npm-shrinkwrap.json": "npm",
	"pnpm-lock.yaml": "pnpm", "yarn.lock": "yarn", "bun.lock": "bun",
	"bun.lockb": "bun", "Cargo.lock": "cargo", "pubspec.lock": "pub",
}

func detectLockfiles(s Snapshot) []Fact {
	var out []Fact
	for name, manager := range lockNames {
		for _, p := range s.Paths(name) {
			hash, err := s.lockDigest(p)
			if err != nil {
				out = append(out, unknown("lockfile", p, "unreadable or oversized lockfile"))
				continue
			}
			f := fact("lockfile", manager, p)
			f.SHA256 = hash
			out = append(out, f)
		}
	}
	return out
}
