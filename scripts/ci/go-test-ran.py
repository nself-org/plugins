#!/usr/bin/env python3
"""Require actual passing go test -json events for every named test prefix.

Fails on: a non-JSON line, any fail event (test or package), a skipped test whose
name starts with a named prefix, and a named prefix with no passing test.
Skips of tests that were not named (and package-level skips such as "no test
files") are ignored, because a package run always carries some of them.
"""
import json
import sys


def check(stream, prefixes):
    passed = {prefix: False for prefix in prefixes}
    bad = False
    for line in stream:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            bad = True
            continue
        action = event.get("Action")
        name = event.get("Test", "")
        if action == "fail":
            bad = True
        if action == "skip" and name and any(name.startswith(p) for p in prefixes):
            bad = True
        if action == "pass" and name:
            for prefix in passed:
                if name.startswith(prefix):
                    passed[prefix] = True
    return not bad and bool(passed) and all(passed.values())


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit("usage: go-test-ran.py TestPrefix [TestPrefix ...]")
    sys.exit(0 if check(sys.stdin, sys.argv[1:]) else 1)
