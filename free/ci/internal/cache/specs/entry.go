package specs

// Purpose: map tar headers to authz.Entry values (the path sanitiser input) and
// share the archive limits between pack and unpack.
// Inputs:  tar.Header. Outputs: authz.Entry, cacheLimits.
// Constraints: only regular files, directories and symlinks map to an admitted
// type; a hard link is flagged so the sanitiser refuses it; every other tar
// type (device, FIFO, pax global header, GNU extensions) maps to an irregular
// mode the sanitiser refuses.

import (
	"archive/tar"
	"io/fs"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// cacheLimits are the extraction limits of a cache archive: the sanitiser
// defaults, with ".git" components admitted (cargo's git checkouts carry them).
func cacheLimits() authz.Limits {
	l := authz.DefaultLimits()
	l.AllowDotGit = true
	return l
}

// CacheLimits returns the shared pack, restore and remote validation limits.
func CacheLimits() authz.Limits { return cacheLimits() }

func entryOf(h *tar.Header, packed int64) authz.Entry {
	e := authz.Entry{Name: h.Name, LinkTarget: h.Linkname, Size: h.Size, Packed: packed, Mode: fs.FileMode(h.Mode) & fs.ModePerm}
	switch h.Typeflag {
	case tar.TypeReg:
	case tar.TypeDir:
		e.Mode |= fs.ModeDir
	case tar.TypeSymlink:
		e.Mode |= fs.ModeSymlink
	case tar.TypeLink:
		e.Hardlink = true
	case tar.TypeChar:
		e.Mode |= fs.ModeDevice | fs.ModeCharDevice
	case tar.TypeBlock:
		e.Mode |= fs.ModeDevice
	case tar.TypeFifo:
		e.Mode |= fs.ModeNamedPipe
	default:
		e.Mode |= fs.ModeIrregular
	}
	if h.Mode&0o7000 != 0 {
		e.Mode |= fs.ModeSetuid // the sanitiser refuses any set-id bit
	}
	return e
}
