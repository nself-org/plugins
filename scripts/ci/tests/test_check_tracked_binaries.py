"""Tests for scripts/ci/check-tracked-binaries.py (P7-GUARD G6 rule, P7-HYG-32).

Each case builds a throwaway git repo, stages one fixture and runs the script
there, so no binary fixture is ever committed to this repository.
Runs under unittest (no third-party packages) and under pytest.
"""
import os
import subprocess
import sys
import tempfile
import unittest

SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..",
                      "check-tracked-binaries.py")


def pe_image(with_signature, e_lfanew=0x80):
    blob = bytearray(b"MZ" + b"\x00" * (e_lfanew + 64))
    blob[0x3C:0x40] = e_lfanew.to_bytes(4, "little")
    if with_signature:
        blob[e_lfanew:e_lfanew + 4] = b"PE\x00\x00"
    return bytes(blob)


class CheckTrackedBinaries(unittest.TestCase):
    def run_check(self, files):
        """files: {path: (bytes, executable)}. Returns (rc, stdout)."""
        with tempfile.TemporaryDirectory() as repo:
            subprocess.run(["git", "init", "-q", repo], check=True)
            for path, (data, executable) in files.items():
                full = os.path.join(repo, path)
                os.makedirs(os.path.dirname(full), exist_ok=True)
                with open(full, "wb") as fh:
                    fh.write(data)
                flag = "--chmod=+x" if executable else "--chmod=-x"
                subprocess.run(["git", "add", flag, "--", path], cwd=repo, check=True)
            proc = subprocess.run([sys.executable, SCRIPT], cwd=repo,
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            return proc.returncode, proc.stdout.decode()

    def assert_hit(self, data, kind, executable=False, path="out/tool"):
        rc, out = self.run_check({path: (data, executable)})
        self.assertEqual(rc, 1, out)
        self.assertEqual(out.strip(), "%s: tracked compiled binary (%s)" % (path, kind))

    def assert_clean(self, data, executable=False):
        rc, out = self.run_check({"out/tool": (data, executable)})
        self.assertEqual((rc, out), (0, ""))

    def test_elf(self):
        self.assert_hit(b"\x7fELF\x02\x01\x01" + b"\x00" * 60, "ELF")

    def test_macho_four_magics(self):
        cases = [
            (b"\xfe\xed\xfa\xce", "Mach-O 32-bit"),
            (b"\xfe\xed\xfa\xcf", "Mach-O 64-bit"),
            (b"\xce\xfa\xed\xfe", "Mach-O 32-bit, little-endian"),
            (b"\xcf\xfa\xed\xfe", "Mach-O 64-bit, little-endian"),
        ]
        for magic, kind in cases:
            with self.subTest(kind=kind):
                self.assert_hit(magic + b"\x07\x00\x00\x01" * 8, kind)

    def test_fat(self):
        self.assert_hit(b"\xca\xfe\xba\xbe\x00\x00\x00\x02" + b"\x00" * 40, "Mach-O fat")

    def test_java_class_is_not_fat(self):
        # 0xCAFEBABE + minor 0 + major 52 (Java 8): not a compiled binary.
        self.assert_clean(b"\xca\xfe\xba\xbe\x00\x00\x00\x34" + b"\x01" * 40)

    def test_pe_with_signature(self):
        self.assert_hit(pe_image(True), "PE")

    def test_pe_without_signature_passes(self):
        self.assert_clean(pe_image(False))

    def test_pe_signature_past_prefix(self):
        self.assert_hit(pe_image(True, e_lfanew=9000), "PE")

    def test_executable_with_nul(self):
        self.assert_hit(b"#!/bin/sh\n\x00\x01\x02", "mode 100755 with NUL byte",
                        executable=True)

    def test_nul_in_non_executable_passes(self):
        self.assert_clean(b"data\x00data", executable=False)

    def test_executable_shell_script_passes(self):
        self.assert_clean(b"#!/bin/sh\necho ok\n", executable=True)

    def test_clean_text_passes(self):
        self.assert_clean(b"package main\n")

    def test_path_with_spaces_and_unicode(self):
        self.assert_hit(b"\x7fELF" + b"\x00" * 20, "ELF", path="dir one/bïn ary")

    def test_each_hit_reported_once_with_others_clean(self):
        rc, out = self.run_check({
            "a/ok.go": (b"package a\n", False),
            "b/elf": (b"\x7fELF" + b"\x00" * 20, False),
            "c/macho": (b"\xcf\xfa\xed\xfe" + b"\x00" * 20, False),
        })
        self.assertEqual(rc, 1)
        self.assertEqual(sorted(out.strip().splitlines()), [
            "b/elf: tracked compiled binary (ELF)",
            "c/macho: tracked compiled binary (Mach-O 64-bit, little-endian)",
        ])


if __name__ == "__main__":
    unittest.main()
