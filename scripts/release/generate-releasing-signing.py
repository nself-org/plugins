#!/usr/bin/env python3
"""Generate the release build-evidence/recovery paragraph in Releasing.md."""
from pathlib import Path
import argparse

ROOT = Path(__file__).resolve().parents[2]
PAGE = ROOT / ".github/wiki/Releasing.md"
START = "<!-- BEGIN GENERATED:release-built-signing -->"
END = "<!-- END GENERATED:release-built-signing -->"
TEXT = """- Before signing any source or platform tarball, the `sign` job requires every published tarball's
  exact `<sha256>  <name>` line in `built-sha256.txt`, the list `publish` wrote of bytes this run built
  and the Release serves unchanged (`scripts/release/platform-built-check.sh`). The publish job never
  uses `--clobber`: a previously uploaded asset wins over this run's build. Without this gate someone
  with `contents: write` could upload source or a binary and receive the official signature.
  A refusal stops signing. Delete each refused tarball AND its `.sha256` from the Release, then re-run
  so this run's build is published. For draft-first releases, let the workflow upload both source and
  platform tarballs, or remove the draft's tarballs and their `.sha256` assets before tagging. A full
  re-run whose rebuild differs from published bytes (for example a changed gzip or Go toolchain)
  requires the same recovery.
"""


def render(text):
    if START in text:
        begin = text.index(START)
        end = text.index(END, begin) + len(END)
    else:
        begin = text.index("- The `sign` job signs a platform tarball only")
        end = text.index("- `release-data`", begin)
        # Preserve the newline before the next list item.
        end -= 1
    return text[:begin] + START + "\n" + TEXT + END + text[end:]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    before = PAGE.read_text()
    after = render(before)
    if args.check:
        if before != after:
            raise SystemExit("Releasing.md signing guidance is stale; run this generator")
    else:
        PAGE.write_text(after)
