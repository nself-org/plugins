package lifecycle

// Purpose: where the agent binary comes from, and the check that it fits the node.
// Inputs:  the node's platform (from `uname -sm`) and a local file path.
// Outputs: a local path of a verified binary; an error when the file is not an ELF (linux) or Mach-O
//          (darwin) executable for the node's architecture.
// Constraints: LocalFile is the only source here, so this Ticket does not depend on the signing chain;
//          P7-NODE-15 extends AgentSource with a signed Release source. The check reads the header
//          only; it proves the format fits the node, not that the binary is trustworthy (the admin
//          chose the file).

import (
	"context"
	"debug/elf"
	"debug/macho"
	"os"
	"strings"
)

// maxAgentBytes bounds an agent binary; the agent is a small static Go binary.
const maxAgentBytes = 256 << 20

// Platform is the node's operating system and architecture in Go spelling.
type Platform struct{ OS, Arch string }

func (p Platform) String() string { return p.OS + "/" + p.Arch }

// ParsePlatform reads `uname -sm` output ("Linux x86_64", "Darwin arm64").
func ParsePlatform(uname string) (Platform, error) {
	f := strings.Fields(uname)
	if len(f) != 2 {
		return Platform{}, usage("cannot read the node platform from %q", uname)
	}
	var p Platform
	switch strings.ToLower(f[0]) {
	case "linux":
		p.OS = "linux"
	case "darwin":
		p.OS = "darwin"
	default:
		return Platform{}, usage("node operating system %q is not supported", f[0])
	}
	switch strings.ToLower(f[1]) {
	case "x86_64", "amd64":
		p.Arch = "amd64"
	case "aarch64", "arm64":
		p.Arch = "arm64"
	default:
		return Platform{}, usage("node architecture %q is not supported", f[1])
	}
	return p, nil
}

// AgentSource yields a local binary for a platform. cleanup may be nil.
type AgentSource interface {
	Fetch(ctx context.Context, p Platform) (path string, cleanup func(), err error)
}

// LocalFile is an agent binary the admin names with --agent-binary.
type LocalFile struct{ Path string }

// Fetch checks the file against the platform and returns it.
func (l LocalFile) Fetch(_ context.Context, p Platform) (string, func(), error) {
	if err := CheckBinary(l.Path, p); err != nil {
		return "", nil, err
	}
	return l.Path, nil, nil
}

// CheckBinary refuses a file that is not a regular executable image for p.
func CheckBinary(path string, p Platform) error {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return usage("agent binary %q is not a readable file", path)
	}
	if st.Size() == 0 || st.Size() > maxAgentBytes {
		return usage("agent binary %q has an unusable size", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return usage("agent binary %q is not a readable file", path)
	}
	defer func() { _ = f.Close() }()
	var magic [4]byte
	if _, err = f.ReadAt(magic[:], 0); err != nil {
		return usage("agent binary %q is not an executable image", path)
	}
	switch {
	case string(magic[:]) == "\x7fELF":
		return checkELF(f, p)
	case isMachO(magic):
		return checkMachO(f, p)
	}
	return usage("agent binary %q is not an ELF or Mach-O executable", path)
}

func isMachO(m [4]byte) bool {
	v := uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])
	switch v {
	case 0xfeedfacf, 0xcffaedfe, 0xcafebabe, 0xbebafeca:
		return true
	}
	return false
}

func checkELF(f *os.File, p Platform) error {
	e, err := elf.NewFile(f)
	if err != nil {
		return usage("agent binary is not a valid ELF file")
	}
	arch := map[elf.Machine]string{elf.EM_X86_64: "amd64", elf.EM_AARCH64: "arm64"}[e.Machine]
	if p.OS != "linux" || arch != p.Arch || e.Type != elf.ET_EXEC && e.Type != elf.ET_DYN {
		return usage("agent binary is for linux/%s, the node is %s", arch, p)
	}
	return nil
}

func checkMachO(f *os.File, p Platform) error {
	want := map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64}[p.Arch]
	if p.OS != "darwin" {
		return usage("agent binary is for darwin, the node is %s", p)
	}
	if m, err := macho.NewFile(f); err == nil {
		if m.Cpu != want {
			return usage("agent binary is for another architecture than the node's %s", p)
		}
		return nil
	}
	fat, err := macho.NewFatFile(f)
	if err != nil {
		return usage("agent binary is not a valid Mach-O file")
	}
	defer func() { _ = fat.Close() }()
	for _, a := range fat.Arches {
		if a.Cpu == want {
			return nil
		}
	}
	return usage("agent binary has no %s slice", p.Arch)
}
