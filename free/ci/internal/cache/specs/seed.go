package specs

// Purpose: the tool-home handling of a spec that rebinds a tool's home
// directory (cargo's CARGO_HOME): copy the user's config into the bound dir,
// and refuse to rebind when the home holds credentials the job would lose.
// Inputs:  the user's home and environment (BeginInput.Home, Getenv).
// Outputs: Session.toolHome, credentialsHidden, seed.
// Constraints: only the files a spec names are copied (never credentials), at
// most 1 MiB each, regular files only, into the pinned cache root. The tool
// home is job-reachable, so every read opens O_NONBLOCK|O_NOFOLLOW, checks the
// opened descriptor is a regular file and reads at most maxSeedBytes: a symlink
// to /dev/zero or a FIFO can neither grow memory nor hang preparation.

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"syscall"
)

// maxSeedBytes bounds what one tool-home file contributes (read or copied).
const maxSeedBytes = 1 << 20

var errUnsafeSeed = errors.New("tool-home file is not a plain regular file within the size bound")

// openSeedFile opens p for a bounded read of a regular file. A missing file
// returns os.ErrNotExist; a symlink, FIFO, device or oversized file returns
// errUnsafeSeed so callers can fail closed.
func openSeedFile(p string) (*os.File, error) {
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, errUnsafeSeed
	}
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > maxSeedBytes {
		_ = f.Close()
		return nil, errUnsafeSeed
	}
	return f, nil
}

// readSeedFile returns the bytes of the regular file p, at most maxSeedBytes.
func readSeedFile(p string) ([]byte, error) {
	f, err := openSeedFile(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxSeedBytes+1))
	if err != nil || len(b) > maxSeedBytes {
		return nil, errUnsafeSeed
	}
	return b, nil
}

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
// credentials the job would lose (cargo's credentials.toml). A config that
// exists but cannot be read safely (symlink, FIFO, device, over 1 MiB) counts
// as hiding credentials: the rebind is refused rather than guessed at.
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
	for _, n := range sd.Names { // a config that names a registry or a token is credentials in all but name
		b, err := readSeedFile(filepath.Join(s.toolHome(sd), n))
		if errors.Is(err, errUnsafeSeed) || (err == nil && cargoReg.Match(b)) {
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
		if f, err := openSeedFile(filepath.Join(home, name)); err == nil {
			if out, err := s.base.OpenFile(path.Join(it.Spec.ID, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
				_, _ = io.Copy(out, io.LimitReader(f, maxSeedBytes))
				_ = out.Close()
				if s.in.Container { // the job UID differs from the agent's
					_ = s.base.Chmod(path.Join(it.Spec.ID, name), 0o644)
				}
			}
			_ = f.Close()
		}
	}
}
