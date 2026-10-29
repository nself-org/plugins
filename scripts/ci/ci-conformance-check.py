#!/usr/bin/env python3
"""Validate the JSON lines of free/ci/scripts/ci-conformance.sh against ci-conformance/v1.

Usage: ci-conformance.sh --sim --json | ci-conformance-check.py [--require-legs A1 A2 ...]

Without --require-legs the stream must hold result lines only (a --list stream is refused):
every leg must pass (a skipped leg is not a pass), and the summary counts must match the lines.
With --require-legs every named leg must appear as a result line or a --list row.
Exit 0 valid, 1 invalid or a leg did not pass, 2 usage.
"""
import json
import re
import sys

LEG = re.compile(r"^(A([1-9]|10)|F[0-9]+)$")
RESULT = {"leg": str, "pass": bool, "reason": str, "evidence_ref": str, "duration_ms": int}
ROW = {"id": str, "kind": str, "path": str}
KINDS = {"sim", "hardware"}


def typed(obj, spec):
    for key, kind in spec.items():
        value = obj.get(key)
        if key not in obj or not isinstance(value, kind) or (kind is int and isinstance(value, bool)):
            return f"{key} is missing or not a {kind.__name__}"
    return None


def check_line(obj):
    """Return (category, problem) for one decoded line."""
    if not isinstance(obj, dict):
        return "bad", "not an object"
    if "summary" in obj:
        s = obj["summary"]
        if not isinstance(s, dict) or typed(s, {"pass": int, "fail": int, "skip": int}) or s.get("kind") not in KINDS:
            return "bad", "malformed summary"
        return "summary", None
    if "leg" in obj:
        why = typed(obj, RESULT)
        if why or not LEG.match(obj["leg"]) or obj.get("kind", "sim") not in KINDS:
            return "bad", why or "bad leg id or kind"
        status = obj.get("status", "pass" if obj["pass"] else "fail")
        if status not in {"pass", "fail", "skip"} or (status == "pass") != obj["pass"]:
            return "bad", "status and pass disagree"
        if obj["duration_ms"] < 0:
            return "bad", "negative duration"
        return "result", None
    if "id" in obj:
        why = typed(obj, ROW)
        if why or not LEG.match(obj["id"]) or obj["kind"] not in KINDS:
            return "bad", why or "bad leg id or kind"
        return "row", None
    return "bad", "neither a result, a row nor a summary"


def check(lines, required):
    problems, results, rows, summaries = [], [], [], []
    for n, line in enumerate(lines, 1):
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            problems.append(f"line {n}: not JSON")
            continue
        cat, why = check_line(obj)
        if cat == "bad":
            problems.append(f"line {n}: {why}")
        elif cat == "result":
            results.append(obj)
        elif cat == "row":
            rows.append(obj)
        else:
            summaries.append(obj["summary"])
    seen = {r["leg"] for r in results} | {r["id"] for r in rows}
    if required:
        for leg in required:
            if leg not in seen:
                problems.append(f"required leg {leg} is absent")
        return problems
    if rows:
        problems.append("a --list stream needs --require-legs")
    if not results:
        problems.append("no leg result lines")
    for r in results:
        if not r["pass"]:
            problems.append(f"leg {r['leg']} did not pass: {r['reason']}")
    if len(summaries) != 1:
        problems.append("want exactly one summary line")
    elif results:
        s = summaries[0]
        want = {"pass": sum(1 for r in results if r["pass"]),
                "skip": sum(1 for r in results if r.get("status") == "skip")}
        want["fail"] = len(results) - want["pass"] - want["skip"]
        if any(s[k] != want[k] for k in want):
            problems.append(f"summary {s} does not match the lines {want}")
    return problems


def main(argv):
    required = []
    if argv:
        if argv[0] != "--require-legs" or len(argv) < 2:
            sys.exit("usage: ci-conformance-check.py [--require-legs LEG ...]")
        required = argv[1:]
    problems = check(sys.stdin, required)
    for p in problems:
        print("ci-conformance-check:", p, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
