#!/usr/bin/env python3
"""check-tracked-binaries.py: fail when a compiled binary is committed.

Purpose
    Keep build outputs out of Git. A tracked entry fails when its first bytes
    are an ELF, Mach-O (32/64-bit, both byte orders, fat) or PE header, or when
    its git mode is 100755 and its first 8000 bytes contain a NUL byte (git's
    own binary heuristic; it catches an executable with no recognisable magic).
    There is no allowlist: a binary test fixture must be generated at test time.

Rule source
    P7-GUARD G6 (hq plan p7, GUARD epic, Decisions G6). cli keeps a Go
    implementation (internal/repoqa/tracked_binaries_test.go, P7-GUARD-09);
    this is the stdlib-Python port for repos with no Go root module. Ticket
    P7-HYG-32 (D-0065).

Inputs
    The Git index of the current directory: `git ls-files -z -s` for the entry
    list and `git cat-file --batch` for blob prefixes. The working tree is never
    read, so an untracked or ignored build output cannot trip the check.
    Submodule entries (160000) and symlinks (120000) are skipped.

Outputs
    One line per hit on stdout: `<path>: tracked compiled binary (<kind>)`.
    Exit 0 when clean, 1 when any hit, 2 when git itself fails.

Notes
    0xCAFEBABE is shared by a Mach-O fat header and a Java class file. Both
    store a 32-bit value next to the magic: nfat_arch (a handful) for fat,
    minor<<16|major (>= 45) for Java. Values 1..30 count as fat; a Java class
    file is not a compiled binary for this rule and passes.
    A PE signature is read at e_lfanew wherever it points (D-0238); a pointer
    past the end of the blob is not a PE, and the bytes before it are streamed
    and dropped, so a huge offset never loads the blob into memory.
"""
import subprocess
import sys

PREFIX_BYTES = 8000
PE_SIGNATURE = b"PE\x00\x00"

ELF_MAGIC = b"\x7fELF"
MACHO_MAGICS = {
    b"\xfe\xed\xfa\xce": "Mach-O 32-bit",
    b"\xfe\xed\xfa\xcf": "Mach-O 64-bit",
    b"\xce\xfa\xed\xfe": "Mach-O 32-bit, little-endian",
    b"\xcf\xfa\xed\xfe": "Mach-O 64-bit, little-endian",
}
FAT_MAGICS = (b"\xca\xfe\xba\xbe", b"\xca\xfe\xba\xbf")
FAT_MAX_ARCHS = 30


def classify_prefix(prefix, mode, read_at):
    """Return the binary kind for a blob prefix, or None when it passes.

    read_at(offset, n) returns the n blob bytes at offset, or b"" when the blob
    ends first; it is only used when a PE header's e_lfanew points past the
    prefix.
    """
    head = prefix[:4]
    if head == ELF_MAGIC:
        return "ELF"
    if head in MACHO_MAGICS:
        return MACHO_MAGICS[head]
    if head in FAT_MAGICS and len(prefix) >= 8:
        if 1 <= int.from_bytes(prefix[4:8], "big") <= FAT_MAX_ARCHS:
            return "Mach-O fat"
    if prefix[:2] == b"MZ" and len(prefix) >= 0x40:
        offset = int.from_bytes(prefix[0x3C:0x40], "little")
        if offset + 4 <= len(prefix):
            signature = prefix[offset:offset + 4]
        else:
            signature = read_at(offset, 4)
        if signature == PE_SIGNATURE:
            return "PE"
    if mode == "100755" and b"\x00" in prefix[:PREFIX_BYTES]:
        return "mode 100755 with NUL byte"
    return None


def tracked_entries():
    """Yield (mode, sha, path) for every index entry."""
    out = subprocess.run(["git", "ls-files", "-z", "-s"], stdout=subprocess.PIPE,
                         check=True).stdout
    for rec in out.split(b"\x00"):
        if not rec:
            continue
        meta, _, path = rec.partition(b"\t")
        mode, sha, _stage = meta.decode().split(" ")
        yield mode, sha, path.decode("utf-8", "surrogateescape")


class BlobReader:
    """Reads blob prefixes through one long-lived `git cat-file --batch`."""

    def __init__(self):
        self.proc = subprocess.Popen(["git", "cat-file", "--batch"],
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE)

    def _open(self, sha):
        """Request a blob; return its size with its bytes next on stdout."""
        self.proc.stdin.write(sha.encode() + b"\n")
        self.proc.stdin.flush()
        header = self.proc.stdout.readline().split()
        if len(header) != 3 or header[1] != b"blob":
            raise RuntimeError("git cat-file: no blob for " + sha)
        return int(header[2])

    def _read(self, n):
        data = self.proc.stdout.read(n)
        if len(data) != n:
            raise RuntimeError("git cat-file: short read")
        return data

    def _drop(self, n):
        while n > 0:
            n -= len(self._read(min(n, 1 << 20)))

    def prefix(self, sha, limit=PREFIX_BYTES):
        size = self._open(sha)
        data = self._read(min(size, limit))
        self._drop(size - len(data) + 1)  # the rest and the trailing newline
        return data

    def read_at(self, sha, offset, n):
        """Return the n bytes at offset, or b"" when the blob ends first."""
        size = self._open(sha)
        if offset + n > size:
            self._drop(size + 1)
            return b""
        self._drop(offset)
        data = self._read(n)
        self._drop(size - offset - n + 1)
        return data

    def close(self):
        self.proc.stdin.close()
        self.proc.wait()


def main():
    hits = []
    reader = BlobReader()
    try:
        for mode, sha, path in tracked_entries():
            if mode in ("160000", "120000"):
                continue
            kind = classify_prefix(reader.prefix(sha), mode,
                                   lambda o, n, s=sha: reader.read_at(s, o, n))
            if kind:
                hits.append("%s: tracked compiled binary (%s)" % (path, kind))
    except (subprocess.CalledProcessError, RuntimeError, OSError) as err:
        print("check-tracked-binaries: git failed: %s" % err, file=sys.stderr)
        return 2
    finally:
        reader.close()
    for line in hits:
        print(line)
    if hits:
        print("%d tracked compiled binar%s; build outputs belong in .gitignore"
              % (len(hits), "y" if len(hits) == 1 else "ies"), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
