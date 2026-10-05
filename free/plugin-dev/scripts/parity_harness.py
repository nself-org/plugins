#!/usr/bin/env python3
"""parity_harness.py: case runner for parity-with-core.sh (P7-CANON-18).

Purpose: run `nself plugin <author command>` (core) and `nself-plugin-dev <command>` (head) on
  identical scripted sandboxes and compare stdout, stderr, exit code, the commands the run executed
  (recorded by stub go, docker, dlv, air) and the file effects (a hash listing of HOME, the working
  directory and TMPDIR after the run).
Inputs:  argv: CORE_BIN HEAD_BIN WORKDIR [--self-test]. Env NO_COLOR is set except in pty cases.
Outputs: one "ok <case>" line per case; "DIFF"/"EMPTY"/"RC"/"EFFECT" blocks; final "parity: PASS (N cases)"
  or FAIL.
Exit:    0 all equal, 1 a difference or a vacuous case, 2 usage.
Constraints: no network (every case is offline; the smoke install/uninstall phases of `test` are not run
  against core because core would call the registry); every case runs in its own sandbox under WORKDIR;
  ~/.nself/logs (core's root command log, written before any plugin code runs) is not part of the effects.
"""
import difflib
import os
import shutil
import sys
import re

import parity_sandbox as sbx
from parity_cases import C, cases
from parity_sandbox import fixture, run_side, setup_stubs, tree_listing

CORE, HEAD, WORK = (os.path.realpath(a) for a in sys.argv[1:4])
SELF_TEST = "--self-test" in sys.argv[4:]
sbx.configure(CORE, HEAD, WORK)

CASES = 0
FAILS = 0


def compare(case, a_side="core", b_side="head", a_case=None):
    global CASES, FAILS
    CASES += 1
    base = os.path.join(WORK, "sb", re.sub(r"[^A-Za-z0-9]+", "_", case["id"]))
    shutil.rmtree(base, ignore_errors=True)
    ra = run_side(a_case or case, a_side, os.path.join(base, "a"))
    rb = run_side(case, b_side, os.path.join(base, "b"))
    bad = False
    for k in ("out", "err", "rc", "cmds", "effects"):
        if ra[k] != rb[k]:
            bad = True
            print("DIFF %s (%s):" % (case["id"], k))
            for i, line in enumerate(difflib.unified_diff(ra[k].splitlines(), rb[k].splitlines(), "core", "head", lineterm="")):
                if i < 40:
                    print(line)
    ne = case.get("nonempty", "out")
    if ne and not ra[ne].strip():
        bad = True
        print("EMPTY %s: nothing captured on %s (vacuous)" % (case["id"], ne))
    if case.get("runs") and not ra["cmds"].strip():
        bad = True
        print("NOCMD %s: no stub command was executed (the case did not reach the command)" % case["id"])
    if case.get("rc") is not None and ra["rc"].strip() != str(case["rc"]):
        bad = True
        print("RC %s: exit code %s, expected %s (the case did not exercise what it claims)" % (case["id"], ra["rc"].strip(), case["rc"]))
    if case.get("effect") and ra["effects"] == run_effects_before(case, base):
        bad = True
        print("EFFECT %s: the run changed nothing on disk (the case did not exercise a file effect)" % case["id"])
    if bad:
        FAILS += 1
    else:
        print("ok " + case["id"])


def run_effects_before(case, base):
    """The effects listing of the untouched fixture (to prove a mutating case really mutated)."""
    root = os.path.join(base, "before")
    fixture(case["fx"], root)
    tokens = [("<S>", root)]
    return "".join("[%s]\n%s" % (n, tree_listing(os.path.join(root, n), tokens)) for n in ("home", "cwd", "tmp"))



def main():
    setup_stubs()
    if SELF_TEST:
        a = C("self identical", ["unlink", "zz"], rc=0)
        compare(a, "core", "core")
        if FAILS:
            print("parity self-test: FAIL (identical sides reported a difference)")
            return 1
        planted = C("self planted", ["unlink", "yy"], rc=0)
        import io, contextlib
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            compare(planted, "core", "core", a_case=C("self planted", ["unlink", "zz"], rc=0))
        if "DIFF self planted (out)" not in buf.getvalue():
            print("parity self-test: FAIL (planted difference not reported)")
            return 1
        # a case that changes nothing on disk must fail the EFFECT check
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            compare(C("self noeffect", ["unlink", "zz"], rc=0, effect=1), "core", "core")
        if "EFFECT self noeffect" not in buf.getvalue():
            print("parity self-test: FAIL (a no-effect case passed the effect check)")
            return 1
        print("parity self-test: PASS (identical sides equal, planted difference reported, no-effect case rejected)")
        return 0
    for c in cases():
        compare(c)
    if CASES == 0:
        print("parity: FAIL (no case ran)")
        return 1
    if FAILS:
        print("parity: FAIL (%d of %d cases differ)" % (FAILS, CASES))
        return 1
    print("parity: PASS (%d cases)" % CASES)
    return 0


sys.exit(main())
