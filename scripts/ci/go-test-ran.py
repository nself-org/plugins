#!/usr/bin/env python3
"""Require actual passing go test -json events for every named test prefix."""
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
        if action in {"skip", "fail"}:
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
