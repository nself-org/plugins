package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// chmodFile widens a written temp file to the wanted mode. It is a seam so a test can
// make it fail the way Colima virtiofs bind mounts do (EPERM).
var chmodFile = func(f *os.File, m os.FileMode) error { return f.Chmod(m) }

// createTemp creates "."+name+".<rand>.tmp" in dir with mode 0644 (before umask), retrying
// on a name collision. No chmod is needed when the umask leaves 0644 intact.
func createTemp(dir, name string) (*os.File, error) {
	var err error
	for i := 0; i < 5; i++ {
		var r [6]byte
		if _, err = rand.Read(r[:]); err != nil {
			return nil, err
		}
		var f *os.File
		f, err = os.OpenFile(filepath.Join(dir, "."+name+"."+hex.EncodeToString(r[:])+".tmp"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return nil, err
}

// writeAtomic writes data to dir/name through a temp file in the same directory and a rename.
// The temp file is created 0644, so chmod runs only when the umask narrowed it; on any failure
// the temp file is removed and the target is left as it was.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := createTemp(dir, name)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if fi, err := tmp.Stat(); err != nil {
		tmp.Close()
		return err
	} else if fi.Mode().Perm() != 0o644 {
		if err := chmodFile(tmp, 0o644); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}
