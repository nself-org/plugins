package main

// Purpose: the tree stream of the job-shim workspace hand-off (D-1230). packTree writes a directory tree
// as a tar stream and unpackTree recreates one, so the agent's workspace reaches the job user's own
// directory (stage-in) and the job's outputs come back (stage-out) without either side opening the
// other's files.
// Inputs:  an os.Root (the side's own directory) and the stream.
// Outputs: directories, regular files and symlinks; nothing else crosses (no devices, fifos, sockets or
//          hard links).
// Constraints: nothing is followed. Walking reports a symlink as a symlink and copies it as one; a file is
//          read only when the opened inode is the regular file that was listed (a swapped-in link is refused); writing goes through the
//          os.Root (a planted link cannot point a write outside it) with O_EXCL, so an existing name, a
//          link included, is never written through. Modes are owner-only (0700 directories, 0600 files,
//          the owner's execute bit kept): no setuid, setgid, sticky, group or world bits ever cross.

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// ownerMode keeps the owner's bits of perm and adds what the owner needs to read, write and enter.
func ownerMode(perm fs.FileMode, dir bool) fs.FileMode {
	if dir {
		return 0o700
	}
	return perm&0o700 | 0o600
}

// packTree writes every directory, regular file and symlink under root to w as a tar stream.
func packTree(root *os.Root, w io.Writer) error {
	tw := tar.NewWriter(w)
	err := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return err
		}
		info, err := root.Lstat(p)
		if err != nil {
			return err
		}
		switch m := info.Mode(); {
		case m.IsDir():
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: p + "/", Mode: int64(ownerMode(m.Perm(), true))})
		case m&fs.ModeSymlink != 0:
			target, err := root.Readlink(p)
			if err != nil {
				return err
			}
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: p, Linkname: target, Mode: 0o777})
		case m.IsRegular():
			return packFile(root, tw, p, info)
		}
		return nil // devices, fifos and sockets do not cross
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// packFile copies one regular file, refusing it when the name now leads to another inode.
func packFile(root *os.Root, tw *tar.Writer, p string, listed fs.FileInfo) error {
	f, err := root.OpenFile(p, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(listed, opened) {
		return fmt.Errorf("%s changed while it was copied", p)
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: p, Size: opened.Size(), Mode: int64(ownerMode(opened.Mode().Perm(), false))}); err != nil {
		return err
	}
	if _, err := io.CopyN(tw, f, opened.Size()); err != nil {
		return fmt.Errorf("%s changed while it was copied: %w", p, err)
	}
	return nil
}

// entryName is the cleaned, local name of a tar entry.
func entryName(raw string) (string, error) {
	name := path.Clean(strings.TrimSuffix(raw, "/"))
	if name == "." || name == "" || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("tree entry %q leaves the directory", raw)
	}
	return name, nil
}

// unpackTree recreates the stream's directories, regular files and symlinks inside root.
func unpackTree(root *os.Root, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := entryName(hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.Mkdir(name, 0o700); err != nil && !existingDir(root, name) {
				return err
			}
		case tar.TypeReg:
			if err := unpackFile(root, name, tr, fs.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := root.Symlink(hdr.Linkname, name); err != nil {
				return err
			}
		}
	}
}

func unpackFile(root *os.Root, name string, r io.Reader, mode fs.FileMode) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, ownerMode(mode.Perm(), false))
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// existingDir reports whether name is already a real directory (not a link to one).
func existingDir(root *os.Root, name string) bool {
	info, err := root.Lstat(name)
	return err == nil && info.IsDir()
}

// emptyTree removes every entry inside root (links are removed, never followed).
func emptyTree(root *os.Root) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := root.RemoveAll(e.Name()); err != nil {
			return err
		}
	}
	return nil
}
