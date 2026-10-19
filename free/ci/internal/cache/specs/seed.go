package specs

// Purpose: the tool-home handling of a spec that rebinds a tool's home
// directory (cargo's CARGO_HOME): copy the user's config into the bound dir,
// and refuse to rebind when the home holds credentials the job would lose.
// Inputs:  the user's home and environment (BeginInput.Home, Getenv).
// Outputs: Session.toolHome, credentialsHidden, seed.
// Constraints: only the files a spec names are copied (never credentials), at
// most 1 MiB each, regular files only, into the pinned cache root.

import (
	"io"
	"os"
	"path"
	"path/filepath"
)

// toolHome is the user's own directory of the tool a seeded spec rebinds.
func (s *Session) toolHome(sd SeedFile) string {
	if home := s.in.Getenv(sd.HomeEnv); home != "" {
		return home
	}
	if s.in.Home != "" {
		return filepath.Join(s.in.Home, sd.HomeDir)
	}
	return ""
}

// credentialsHidden reports whether sp rebinds a tool home that holds
// credentials the job would lose (cargo's credentials.toml).
func (s *Session) credentialsHidden(sp Spec) bool {
	sd := sp.Seed
	if sd.HomeEnv == "" || s.toolHome(sd) == "" {
		return false
	}
	for _, n := range sd.Blockers {
		if _, err := os.Lstat(filepath.Join(s.toolHome(sd), n)); err == nil {
			return true
		}
	}
	return false
}

// seed copies the user's tool-home config files into it.Root (cargo's
// config.toml) because the bound variable hides the user's own directory.
func (s *Session) seed(it *Item) {
	sd := it.Spec.Seed
	home := s.toolHome(sd)
	if sd.HomeEnv == "" || home == "" {
		return
	}
	for _, name := range sd.Names {
		src := filepath.Join(home, name)
		if info, err := os.Lstat(src); err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			continue
		}
		if f, err := os.Open(src); err == nil {
			if out, err := s.base.OpenFile(path.Join(it.Spec.ID, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
				_, _ = io.Copy(out, f)
				_ = out.Close()
			}
			_ = f.Close()
		}
	}
}
