package tart

// Purpose: move the workspace into the VM and the declared outputs back out, with no host mount.
// Inputs:  the executor's pinned workspace root (os.Root), the job's declared artifact paths.
// Outputs: a tar stream of the workspace for `tar -xf -` in the VM; declared paths extracted into the workspace.
// Constraints: the host side never reads outside the workspace root and never follows a link out of it; what
//          comes back from the VM is untrusted: only regular files and directories whose cleaned name is a
//          declared path or below one are written, through the root, with a total size cap; every other entry
//          type (symlink, hard link, device) and every name that climbs or is absolute fails the attempt.

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Bounds on what one attempt may bring back from the VM: bytes of file data, entries and directory depth.
var (
	maxCollectBytes   int64 = 1 << 30
	maxCollectEntries       = 100000
	maxCollectDepth         = 64
)

// Frame markers of the output stream; ssh noise outside them is ignored.
const (
	beginMark = "NSELF-ARTIFACTS-BEGIN"
	endMark   = "NSELF-ARTIFACTS-END"
)

// tarWorkspace writes the workspace as a tar stream. Special files are skipped.
func tarWorkspace(ws *os.Root, w io.Writer) error {
	fsys := ws.FS()
	tw := tar.NewWriter(w)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		var link string
		switch {
		case mode.IsRegular(), mode.IsDir():
		case mode&fs.ModeSymlink != 0:
			if link, err = fs.ReadLink(fsys, p); err != nil {
				return err
			}
		default:
			return nil
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = p
		if mode.IsDir() {
			h.Name += "/"
		}
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if !mode.IsRegular() {
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// cleanArtifactPaths validates the declared output paths (relative, inside the workspace, no globs).
func cleanArtifactPaths(declared []string) ([]string, error) {
	var out []string
	for _, d := range declared {
		c := path.Clean(strings.TrimSpace(d))
		if d == "" || c == "." || path.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") || strings.ContainsAny(c, "*?[\x00\n") {
			return nil, fmt.Errorf("artifact path %q cannot be collected from a VM", d)
		}
		out = append(out, c)
	}
	return out, nil
}

// collectCommand is the remote command that prints the declared paths as a framed base64 tar.
func collectCommand(base string, paths []string) string {
	var q []string
	for _, p := range paths {
		q = append(q, shQuote(p))
	}
	script := "set -o pipefail; cd " + shQuote(base) + " && echo " + beginMark +
		" && COPYFILE_DISABLE=1 tar -cf - -- " + strings.Join(q, " ") + " 2>/dev/null | base64 | fold -w 76; echo " + endMark + " $?"
	return "/bin/bash -c " + shQuote(script)
}

// framedPayload returns the base64 text between the markers of r, and the status printed after the end marker.
func framedPayload(r io.Reader) (io.Reader, func() (int, error)) {
	pr, pw := io.Pipe()
	status := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		in, ended := false, false
		code := -1
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			switch {
			case line == beginMark:
				in = true
			case strings.HasPrefix(line, endMark+" "):
				fmt.Sscanf(strings.TrimPrefix(line, endMark+" "), "%d", &code)
				in, ended = false, true
			case in:
				if _, err := io.WriteString(pw, line); err != nil {
					return
				}
			}
		}
		err := sc.Err()
		if err == nil && !ended {
			err = errors.New("the VM did not finish the output stream")
		}
		_ = pw.CloseWithError(err)
		status <- struct {
			code int
			err  error
		}{code, err}
	}()
	return pr, func() (int, error) { s := <-status; return s.code, s.err }
}

// extractArtifacts unpacks the base64 tar from r into ws, allowing only the declared paths.
func extractArtifacts(r io.Reader, ws *os.Root, allowed []string, limit int64) error {
	tr := tar.NewReader(base64.NewDecoder(base64.StdEncoding, r))
	var total int64
	entries := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("collect: %w", err)
		}
		if entries++; entries > maxCollectEntries {
			return fmt.Errorf("collect: more than %d entries", maxCollectEntries)
		}
		name := path.Clean(strings.TrimSuffix(h.Name, "/"))
		if strings.Count(name, "/") > maxCollectDepth {
			return fmt.Errorf("collect: entry %q is nested deeper than %d", h.Name, maxCollectDepth)
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || !underAny(name, allowed) {
			return fmt.Errorf("collect: entry %q is outside the declared artifact paths", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := ws.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := ws.MkdirAll(path.Dir(name), 0o755); err != nil {
				return err
			}
			perm := os.FileMode(0o644)
			if h.FileInfo().Mode()&0o111 != 0 {
				perm = 0o755
			}
			f, err := ws.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, io.LimitReader(tr, limit-total+1))
			total += n
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			if total > limit {
				return fmt.Errorf("collect: the outputs exceed %d bytes", limit)
			}
		default:
			return fmt.Errorf("collect: entry %q has an unsupported type", h.Name)
		}
	}
}

func underAny(name string, allowed []string) bool {
	for _, a := range allowed {
		if name == a || strings.HasPrefix(name, a+"/") {
			return true
		}
	}
	return false
}

// limitFile writes to f until max bytes, then drops the rest, records that the cap was hit and cancels the run
// that feeds it, so a VM that streams forever fills nothing on the host.
type limitFile struct {
	f        *os.File
	max, n   int64
	cancel   context.CancelFunc
	exceeded bool
}

func (l *limitFile) Write(p []byte) (int, error) {
	if l.exceeded {
		return len(p), nil
	}
	if room := l.max - l.n; int64(len(p)) > room {
		l.exceeded = true
		l.cancel()
		if room > 0 {
			_, _ = l.f.Write(p[:room])
		}
		return len(p), nil
	}
	l.n += int64(len(p))
	return l.f.Write(p)
}
