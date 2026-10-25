"""Tests for version_is_newer in scripts/verify-published-checksums-lib.sh (D-1106).

SemVer precedence, including numeric identifiers too long for a double. No network, no gh.
"""

from pathlib import Path
import subprocess
import unittest

LIB = Path(__file__).resolve().parents[2] / "verify-published-checksums-lib.sh"

# (newer, older): version_is_newer(newer, older) must hold and version_is_newer(older, newer) must not.
ORDERED = [
    ("1.2.4", "1.2.3"),
    ("1.10.0", "1.9.9"),
    ("1.0.0", "1.0.0-rc.1"),
    ("1.0.0-rc.2", "1.0.0-rc.1"),
    ("1.0.0-rc.10", "1.0.0-rc.9"),
    ("1.0.0-rc.1", "1.0.0-beta.9"),
    ("1.0.0-alpha.1", "1.0.0-alpha"),
    ("1.0.0-alpha", "1.0.0-1"),
    ("1.0.0-beta.1", "1.0.0-beta"),
    ("1.0.1", "1.0.0+build.9"),
    # Numeric identifiers beyond 2^53: floats would read these as equal.
    ("1.0.0-9007199254740993", "1.0.0-9007199254740992"),
    ("1.0.0-12345678901234567891", "1.0.0-12345678901234567890"),
    ("1.0.0-100000000000000000000", "1.0.0-99999999999999999999"),
    ("1.0.0-beta.9007199254740993", "1.0.0-beta.9007199254740992"),
    # The same for a long core component.
    ("1.9007199254740993.0", "1.9007199254740992.0"),
    ("2.0.0", "1.99999999999999999999.0"),
]
EQUAL = [
    ("1.0.0", "1.0.0"),
    ("1.0.0+a", "1.0.0+b"),
    ("1.0", "1.0.0"),
    ("1.0.0-12345678901234567890", "1.0.0-12345678901234567890"),
    ("1.0.0-007", "1.0.0-7"),
    ("1.9007199254740993.0", "1.9007199254740993.0"),
]


def newer(a, b):
    r = subprocess.run(["bash", "-c", 'source "$1"; version_is_newer "$2" "$3"', "_", str(LIB), a, b], capture_output=True)
    if r.returncode not in (0, 1):
        raise AssertionError("version_is_newer %s %s: rc %d: %s" % (a, b, r.returncode, r.stderr.decode()))
    return r.returncode == 0


class VersionIsNewer(unittest.TestCase):
    def test_ordered_pairs(self):
        for hi, lo in ORDERED:
            with self.subTest(hi=hi, lo=lo):
                self.assertTrue(newer(hi, lo), "%s should be newer than %s" % (hi, lo))
                self.assertFalse(newer(lo, hi), "%s should not be newer than %s" % (lo, hi))

    def test_equal_pairs_are_not_newer(self):
        for a, b in EQUAL:
            with self.subTest(a=a, b=b):
                self.assertFalse(newer(a, b))
                self.assertFalse(newer(b, a))


if __name__ == "__main__":
    unittest.main()
