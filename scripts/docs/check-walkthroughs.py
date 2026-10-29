#!/usr/bin/env python3
"""Check the CI walkthrough pages against the generated reference and the test suite.

Purpose: every `nself ci ...` line in a code block of a walkthrough page names a command that the generated
         Reference.md lists, every `--flag` on a bare `nself ci` line is a flag that table lists, every
         ```text block that contains `nself ci` carries a `<!-- proven-by: TestName -->` line directly above
         it, every named proof exists in the repository, and the page has a "Proven by" table that lists
         each proof its text blocks name.
Inputs:  page paths, or --self-test; --reference overrides the Reference.md path (default: the generated page
         beside the walkthroughs). Proofs are looked up with `git grep` in the repository that holds the
         script: a Go test name must match `func Name(`, a path must be a tracked file.
Outputs: one line per finding on stdout; exit 0 clean, 1 findings, 2 usage or unreadable input.
Constraints: stdlib only; read-only; no network.
"""

import argparse
import os
import re
import subprocess
import sys

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
DEFAULT_REFERENCE = os.path.join(REPO, ".github", "wiki", "guides", "ci", "Reference.md")
TESTDATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "testdata", "walkthroughs")

FENCE = re.compile(r"^\s*(`{3,})\s*([A-Za-z0-9_+-]*)")
PROVEN_BY = re.compile(r"^\s*<!--\s*proven-by:\s*([A-Za-z0-9_./-]+)\s*-->\s*$")
COMMAND_ROW = re.compile(r"^\|\s*`(nself ci(?: [a-z][a-z0-9-]*)*)`\s*\|")
FLAG_ROW = re.compile(r"^\|\s*`(--[a-z0-9][a-z0-9-]*)`\s*\|")
WORD = re.compile(r"^[a-z][a-z0-9-]*$")


def load_reference(path):
    """Return (commands, flags) from the generated command and flag tables of Reference.md."""
    commands, flags = set(), set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            m = COMMAND_ROW.match(line)
            if m:
                commands.add(m.group(1))
                continue
            m = FLAG_ROW.match(line)
            if m:
                flags.add(m.group(1))
    if not commands:
        raise ValueError("%s: no command rows found" % path)
    return commands, flags


def code_blocks(lines):
    """Yield (language, fence line index, body lines as (index, text)) for each fenced block."""
    i = 0
    while i < len(lines):
        m = FENCE.match(lines[i])
        if not m:
            i += 1
            continue
        ticks, lang, start = m.group(1), m.group(2), i
        body = []
        i += 1
        while i < len(lines) and lines[i].strip() != ticks and not (
            lines[i].strip().startswith(ticks) and lines[i].strip().strip("`") == ""
        ):
            body.append((i, lines[i]))
            i += 1
        yield lang, start, body
        i += 1


def ci_tokens(text):
    """Return the tokens after `nself ci` when the line is an nself ci command, else None."""
    s = text.strip()
    if s.startswith("$ "):
        s = s[2:].lstrip()
    parts = s.split()
    if len(parts) < 2 or parts[0] != "nself" or parts[1] != "ci":
        return None
    return parts[2:]


def check_command(tokens, commands, flags):
    """Return a finding for one `nself ci` line, or None."""
    words = []
    for tok in tokens:
        if not WORD.match(tok):
            break
        words.append(tok)
    path = "nself ci"
    used = 0
    for n in range(len(words), 0, -1):
        cand = "nself ci " + " ".join(words[:n])
        if cand in commands:
            path, used = cand, n
            break
    if used < len(words):
        nxt = path + " " + words[used]
        if any(c == nxt or c.startswith(nxt + " ") for c in commands):
            k = used + 1
            while k < len(words):
                head = "nself ci " + " ".join(words[: k + 1])
                if not any(c == head or c.startswith(head + " ") for c in commands):
                    break
                k += 1
            return "`%s` is not a command in Reference.md" % " ".join(["nself ci"] + words[: k + 1])
        if path == "nself ci":
            # A bare run takes flags and paths, never a plain word: a leading word is a misspelt verb.
            return "`nself ci %s` is not a command in Reference.md" % words[0]
    if path == "nself ci":
        for tok in tokens:
            if tok == "--":
                break
            if tok.startswith("--"):
                name = tok.split("=", 1)[0]
                if name not in flags:
                    return "`%s` is not a flag listed in Reference.md" % name
    return None


def proof_exists(name, cache):
    """Report whether a proof (a Go test name or a repo path) exists in the repository."""
    if name in cache:
        return cache[name]
    if "/" in name:
        args = ["git", "-C", REPO, "ls-files", "--error-unmatch", "--", name]
    else:
        args = ["git", "-C", REPO, "grep", "-q", "-F", "func %s(" % name, "--", "*.go"]
    ok = subprocess.run(args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
    cache[name] = ok
    return ok


def proven_by_table(lines):
    """Return the proof names in the first column of the "## Proven by" table, or None without the section."""
    names, inside = [], False
    for line in lines:
        if line.startswith("## "):
            inside = line.strip() == "## Proven by"
            continue
        if inside and line.startswith("|"):
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            for m in re.finditer(r"`([A-Za-z0-9_./-]+)`", cells[-1] if len(cells) > 1 else ""):
                names.append(m.group(1))
    return names if any(l.strip() == "## Proven by" for l in lines) else None


def check_page(path, commands, flags, cache):
    """Return the findings for one page as strings."""
    try:
        with open(path, encoding="utf-8") as f:
            lines = f.read().split("\n")
    except OSError as e:
        return ["%s: %s" % (path, e)]
    out = []
    used_proofs = []
    for lang, start, body in code_blocks(lines):
        has_ci = False
        for idx, text in body:
            tokens = ci_tokens(text)
            if tokens is None:
                continue
            has_ci = True
            msg = check_command(tokens, commands, flags)
            if msg:
                out.append("%s:%d: %s" % (path, idx + 1, msg))
        if lang == "text" and has_ci:
            m = PROVEN_BY.match(lines[start - 1]) if start > 0 else None
            if not m:
                out.append("%s:%d: text block with `nself ci` has no proven-by line directly above it" % (path, start + 1))
                continue
            used_proofs.append((start, m.group(1)))
    table = proven_by_table(lines)
    if table is None:
        out.append("%s: no \"## Proven by\" section" % path)
        table = []
    for name in sorted(set(table) | {n for _, n in used_proofs}):
        if not proof_exists(name, cache):
            out.append("%s: proof %s does not exist in the repository" % (path, name))
    for start, name in used_proofs:
        if name not in table:
            out.append("%s:%d: proof %s is missing from the Proven by table" % (path, start, name))
    return out


def self_test():
    """Run the fixtures: good.md must pass and every bad-*.md must fail. Return the exit code."""
    ref = os.path.join(TESTDATA, "Reference.md")
    commands, flags = load_reference(ref)
    cache, status = {}, 0
    for name in sorted(os.listdir(TESTDATA)):
        if not name.endswith(".md") or name == "Reference.md":
            continue
        findings = check_page(os.path.join(TESTDATA, name), commands, flags, cache)
        want_fail = name.startswith("bad-")
        if want_fail and not findings:
            print("self-test: %s passed but must fail" % name)
            status = 1
        elif not want_fail and findings:
            print("self-test: %s must pass:\n  %s" % (name, "\n  ".join(findings)))
            status = 1
        else:
            print("self-test: %s ok (%s)" % (name, findings[0] if findings else "clean"))
    return status


def main(argv):
    ap = argparse.ArgumentParser(description="Check CI walkthrough pages.")
    ap.add_argument("pages", nargs="*")
    ap.add_argument("--reference", default=DEFAULT_REFERENCE)
    ap.add_argument("--self-test", action="store_true")
    a = ap.parse_args(argv)
    if a.self_test:
        return self_test()
    if not a.pages:
        ap.print_usage(sys.stderr)
        return 2
    try:
        commands, flags = load_reference(a.reference)
    except (OSError, ValueError) as e:
        print(e, file=sys.stderr)
        return 2
    cache, findings = {}, []
    for page in a.pages:
        findings += check_page(page, commands, flags, cache)
    for f in findings:
        print(f)
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
