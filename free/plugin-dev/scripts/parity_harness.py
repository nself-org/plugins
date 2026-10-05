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
import hashlib
import os
import re
import shutil
import stat
import subprocess
import sys

CORE, HEAD, WORK = (os.path.realpath(a) if i < 3 else a for i, a in enumerate(sys.argv[1:4]))
SELF_TEST = "--self-test" in sys.argv[4:]
STUBS = os.path.join(WORK, "stubs")
SYSPATH = "/usr/bin:/bin"

STUB_SCRIPT = r'''#!/bin/sh
name=$(basename "$0")
printf '%s %s | cwd=%s | dev=%s\n' "$name" "$*" "$(pwd -P)" "${NSELF_PLUGIN_DEV:-}" >> "$STUB_LOG"
case "$name" in
  go)
    if [ -n "${STUB_FAIL_GO:-}" ]; then echo "go: boom" >&2; exit 2; fi
    prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then : > "$a"; fi; prev=$a; done
    echo "go stub ok";;
  docker)
    if [ "$1" = "info" ]; then exit "${STUB_DOCKER_INFO_RC:-0}"; fi
    if [ -n "${STUB_FAIL_GO:-}" ]; then echo "docker: boom" >&2; exit 2; fi
    echo "docker stub ok";;
  dlv) echo "dlv stub ok"; exit "${STUB_DLV_RC:-0}";;
  air) echo "air stub ok"; exit "${STUB_AIR_RC:-0}";;
esac
exit 0
'''


def setup_stubs():
    for d in ("stubs", "stubs-nodlv"):
        os.makedirs(os.path.join(WORK, d), exist_ok=True)
    for n in ("go", "docker", "dlv", "air"):
        for d in ("stubs",) if n in ("dlv", "air") else ("stubs", "stubs-nodlv"):
            p = os.path.join(WORK, d, n)
            with open(p, "w") as f:
                f.write(STUB_SCRIPT)
            os.chmod(p, 0o755)


# ---- sandboxes ----------------------------------------------------------------------------------
def w(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(text)


def fixture(name, root):
    """Populate home/ cwd/ tmp/ under root for the named fixture."""
    home, cwd = os.path.join(root, "home"), os.path.join(root, "cwd")
    for d in ("home", "cwd", "tmp"):
        os.makedirs(os.path.join(root, d), exist_ok=True)
    plug = lambda base: (w(os.path.join(base, "plugin.yaml"), "name: fxplug\nversion: 0.0.1\n"),
                         w(os.path.join(base, "cmd", "main.go"), "package main\nfunc main() {}\n"))
    if "plugin" in name:
        plug(os.path.join(cwd, "fxplug"))
    if "incwd" in name:
        plug(cwd)
    if "quoted" in name:
        w(os.path.join(cwd, "q", "plugin.yaml"), "name: \"quoted-name\"\n")
    if "emptyname" in name:
        w(os.path.join(cwd, "en", "plugin.yaml"), "name:\nversion: 1\n")
    if "bare" in name:
        os.makedirs(os.path.join(cwd, "bare"), exist_ok=True)
    if "afile" in name:
        w(os.path.join(cwd, "afile"), "x")
    if "existing" in name:
        w(os.path.join(cwd, "demo", "keep.txt"), "keep\n")
    if "linked" in name:
        w(os.path.join(home, ".nself", "plugin-links.json"),
          '{\n  "fxplug": "%s/cwd/fxplug",\n  "other": "%s/elsewhere"\n}' % (root, root))
    if "linkone" in name:
        w(os.path.join(home, ".nself", "plugin-links.json"), '{"solo": "%s/solo"}' % root)
    if "badjson" in name:
        w(os.path.join(home, ".nself", "plugin-links.json"), "{not json")
    if "nullinks" in name:
        w(os.path.join(home, ".nself", "plugin-links.json"), "{}")
    if "dotnselffile" in name:
        w(os.path.join(home, ".nself"), "i am a file")
    if "linkedgone" in name:
        w(os.path.join(home, ".nself", "plugin-links.json"), '{"ghost": "%s/gone"}' % root)
    os.makedirs(os.path.join(cwd, "sub"), exist_ok=True)
    return home, cwd


def tree_listing(root, tokens):
    """Sorted listing of every entry under root: path, type, mode, content hash (tokens normalised)."""
    out = []
    for dp, dns, fns in os.walk(root):
        dns[:] = sorted(d for d in dns if not (dp.endswith("/.nself") and d == "logs"))
        for n in sorted(fns + [d for d in dns if os.path.islink(os.path.join(dp, d))]):
            p = os.path.join(dp, n)
            rel = os.path.relpath(p, root)
            st = os.lstat(p)
            mode = oct(stat.S_IMODE(st.st_mode))
            if stat.S_ISLNK(st.st_mode):
                out.append("L %s %s -> %s" % (rel, mode, norm_tokens(os.readlink(p), tokens)))
            else:
                data = open(p, "rb").read()
                try:
                    data = norm_tokens(data.decode("utf-8"), tokens).encode("utf-8")
                except UnicodeDecodeError:
                    pass
                out.append("F %s %s %d %s" % (rel, mode, len(data), hashlib.sha256(data).hexdigest()[:16]))
        for d in dns:
            if not os.path.islink(os.path.join(dp, d)):
                p = os.path.join(dp, d)
                out.append("D %s %s" % (os.path.relpath(p, root), oct(stat.S_IMODE(os.lstat(p).st_mode))))
    # core's root creates ~/.nself only to write logs/; an otherwise empty ~/.nself is not an effect of the command
    if not any(e.split(" ")[1].startswith(".nself/") for e in out if e[0] in "FL"):
        sub = [e for e in out if e.startswith("D .nself/") and not e.startswith("D .nself/logs")]
        if not sub:
            out = [e for e in out if not e.startswith("D .nself ")]
    return "\n".join(sorted(out)) + "\n"


def norm_tokens(s, tokens):
    for tok, val in tokens:
        s = s.replace(val, tok)
    return s


def norm_text(s, tokens):
    s = norm_tokens(s, tokens)
    s = s.replace("nself plugin-dev ", "nself plugin ")
    s = re.sub(r"\[[0-9]+\.[0-9]s\]", "[T]", s)
    s = re.sub(r"(Launching debugger via )\S+ (?:plugin )?debug ", r"\1<EXE> debug ", s)
    s = re.sub(r"^(Files created: )(.*)$", lambda m: m.group(1) + ", ".join(sorted(m.group(2).split(", "))), s, flags=re.M)
    return s


def run_side(case, side, root):
    home, cwd = fixture(case["fx"], root)
    if side != "core" and not os.path.exists(os.path.join(home, ".nself")):
        # the mounting nself root creates ~/.nself/logs (0755) before the plugin runs; core's own run does the same
        os.makedirs(os.path.join(home, ".nself", "logs"))
    exe = CORE if side == "core" else HEAD
    argv = [a.replace("{cwd}", cwd).replace("{home}", home).replace("{S}", root) for a in case["argv"]]
    cmd = ([exe, "plugin"] if side == "core" else [exe]) + argv
    log = os.path.join(root, "stub.log")
    open(log, "w").close()
    env = {"HOME": home, "TMPDIR": os.path.join(root, "tmp"), "STUB_LOG": log,
           "PATH": (STUBS if case.get("path", "stubs") == "stubs" else os.path.join(WORK, "stubs-nodlv")) + ":" + SYSPATH}
    if not case.get("pty"):
        env["NO_COLOR"] = "1"
    env.update(case.get("env", {}))
    workdir = os.path.join(cwd, case["cwd"]) if case.get("cwd") else cwd
    stdin = case.get("stdin", "null")
    if case.get("pty"):
        script = ("import os,pty,sys\npid,fd=pty.fork()\n"
                  "if pid==0:\n os.execvpe(sys.argv[1],sys.argv[1:],os.environ)\n"
                  "buf=b''\nwhile True:\n try:\n  d=os.read(fd,4096)\n except OSError:\n  break\n if not d: break\n buf+=d\n"
                  "_,st=os.waitpid(pid,0)\nsys.stdout.buffer.write(buf)\nsys.exit(os.waitstatus_to_exitcode(st))\n")
        r = subprocess.run([sys.executable, "-c", script] + cmd, cwd=workdir, env=env, capture_output=True, timeout=60)
        out, err = r.stdout.decode().replace("\r\n", "\n"), ""
        rc = r.returncode
    else:
        sin = {"null": subprocess.DEVNULL}.get(stdin)
        kw = {"stdin": sin} if sin is not None else {"input": stdin[5:].encode()}
        r = subprocess.run(cmd, cwd=workdir, env=env, capture_output=True, timeout=120, **kw)
        out, err, rc = r.stdout.decode(), r.stderr.decode(), r.returncode
    tokens = [(("<S>"), root), ("<EXE>", CORE), ("<EXE>", HEAD)]
    res = {
        "out": norm_text(out, tokens), "err": norm_text(err, tokens), "rc": str(rc) + "\n",
        "cmds": norm_text(open(log).read(), tokens),
        "effects": "".join("[%s]\n%s" % (n, tree_listing(os.path.join(root, n), tokens)) for n in ("home", "cwd", "tmp")),
    }
    if case.get("sorted"):
        for k in ("out", "err"):
            res[k] = "".join(sorted(res[k].splitlines(True)))
    return res


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


def C(id, argv, fx="plain", **kw):
    d = {"id": id, "argv": argv, "fx": fx}
    d.update(kw)
    return d


def cases():
    cs = []
    # ---- help ----
    for c in ("init", "new", "scaffold", "dev", "debug", "link", "unlink", "test"):
        cs.append(C("help " + c, [c, "--help"]))
    cs.append(C("help -h", ["init", "-h"]))
    cs.append(C("help after args", ["debug", "x", "--help"]))
    # ---- init / new ----
    base = ["demo", "--no-interactive"]
    cs.append(C("init default", ["init"] + base, rc=0, effect=1))
    cs.append(C("new default", ["new"] + base, rc=0, effect=1))
    cs.append(C("scaffold alias", ["scaffold"] + base, rc=0, effect=1))
    for t in ("rust", "node", "static"):
        cs.append(C("init template " + t, ["init"] + base + ["--template", t], rc=0, effect=1))
    cs.append(C("init pro bundle", ["init"] + base + ["--tier", "pro", "--bundle", "nClaw", "--category", "ai",
                                                     "--author", "A B", "--description", "d d", "--port", "9090",
                                                     "--min-cli", "1.2.3", "--min-sdk", "0.2.0"], rc=0, effect=1))
    for t in ("app-isolation", "cloud-tenant", "both", "none"):
        cs.append(C("init tenancy " + t, ["init", "demo", "--tenancy", t], rc=0, effect=1))
    cs.append(C("init out dir", ["init", "demo", "--no-interactive", "--out", "{cwd}/elsewhere/x"], rc=0, effect=1))
    cs.append(C("init out equals form", ["init", "demo", "--no-interactive", "--out={cwd}/o2", "--port=7000"], rc=0, effect=1))
    cs.append(C("init flags before name", ["init", "--no-interactive", "--tier", "free", "demo"], rc=0, effect=1))
    cs.append(C("init after double dash", ["init", "--no-interactive", "--", "demo"], rc=0, effect=1))
    cs.append(C("init global flags", ["init", "demo", "--no-interactive", "--no-monorepo", "--no-deprecation-warnings"], rc=0, effect=1))
    cs.append(C("init existing nonempty", ["init", "demo", "--no-interactive"], fx="existing", rc=1, nonempty="err"))
    cs.append(C("init existing force", ["init", "demo", "--no-interactive", "--force"], fx="existing", rc=0, effect=1))
    cs.append(C("init stdin pipe no prompt", ["init", "demo"], stdin="pipe:y\nb\n", rc=0, effect=1))
    cs.append(C("init stdin devnull prompts", ["init", "demo"], rc=1, nonempty="err"))
    for id, a in (("bad template", ["init", "demo", "--template", "bad", "--no-interactive"]),
                  ("bad tenancy", ["init", "demo", "--tenancy", "x"]),
                  ("bad tier", ["init", "demo", "--tier", "gold", "--no-interactive"]),
                  ("bad name upper", ["init", "Demo", "--no-interactive"]),
                  ("bad name short", ["init", "x", "--no-interactive"]),
                  ("bad name hyphen end", ["init", "demo-", "--no-interactive"]),
                  ("no arg", ["init"]), ("two args", ["init", "a", "b"]),
                  ("unknown flag", ["init", "demo", "--bogus"]), ("unknown flag value", ["init", "--bogus=1", "demo"]),
                  ("unknown shorthand", ["init", "demo", "-z"]),
                  ("bad port", ["init", "demo", "--port", "abc"]), ("port range", ["init", "demo", "--port", "99999999999999999999"]),
                  ("missing flag arg", ["init", "demo", "--out"]), ("bad bool", ["init", "demo", "--force=maybe"])):
        cs.append(C("init err " + id, a, rc=1, nonempty="err"))
    cs.append(C("new no arg", ["new"], rc=1, nonempty="err"))
    cs.append(C("new bad flag", ["new", "demo", "--bogus"], rc=1, nonempty="err"))
    cs.append(C("new bad template", ["new", "demo", "--template", "bad"], rc=1, nonempty="err"))
    # ---- link / unlink ----
    cs.append(C("link ok", ["link", "{cwd}/fxplug"], fx="plugin", rc=0, effect=1))
    cs.append(C("link relative", ["link", "fxplug", "--host"], fx="plugin", rc=0, effect=1))
    cs.append(C("link quoted name", ["link", "q"], fx="quoted", rc=0, effect=1))
    cs.append(C("link empty name falls back", ["link", "en"], fx="emptyname", rc=0, effect=1))
    cs.append(C("link adds to existing", ["link", "fxplug"], fx="plugin-linked", rc=0, effect=1))
    cs.append(C("link missing path", ["link", "{cwd}/nope"], rc=1, nonempty="err"))
    cs.append(C("link a file", ["link", "afile"], fx="afile", rc=1, nonempty="err"))
    cs.append(C("link no plugin.yaml", ["link", "bare"], fx="bare", rc=1, nonempty="err"))
    cs.append(C("link bad json", ["link", "fxplug"], fx="plugin-badjson", rc=1, nonempty="err"))
    cs.append(C("link unwritable nself", ["link", "fxplug"], fx="plugin-dotnselffile", rc=1, nonempty="err"))
    cs.append(C("link no arg", ["link"], rc=1, nonempty="err"))
    cs.append(C("link list empty", ["link", "--list", "x"], rc=0))
    cs.append(C("link list one", ["link", "--list", "x"], fx="linkone", rc=0))
    cs.append(C("link list two sorted", ["link", "--list", "x"], fx="linked", rc=0, sorted=True))
    cs.append(C("unlink linked", ["unlink", "fxplug"], fx="linked", rc=0, effect=1))
    cs.append(C("unlink not linked", ["unlink", "zzz"], rc=0))
    cs.append(C("unlink bad json", ["unlink", "zzz"], fx="badjson", rc=1, nonempty="err"))
    cs.append(C("unlink no arg", ["unlink"], rc=1, nonempty="err"))
    cs.append(C("link then color", ["link", "{cwd}/fxplug"], fx="plugin", pty=1, rc=0, effect=1))
    cs.append(C("unlink color", ["unlink", "zzz"], pty=1, rc=0))
    # ---- debug ----
    cs.append(C("debug port-only auto", ["debug", "x", "--port-only"], rc=0))
    cs.append(C("debug port-only manual", ["debug", "x", "--port", "3000", "--port-only"], rc=0))
    cs.append(C("debug port low", ["debug", "x", "--port", "80"], rc=1, nonempty="err"))
    cs.append(C("debug port edge 1023", ["debug", "x", "--port", "1023", "--port-only"], rc=1, nonempty="err"))
    cs.append(C("debug port edge 1024", ["debug", "x", "--port", "1024", "--port-only"], rc=0))
    cs.append(C("debug port high", ["debug", "x", "--port", "70000"], rc=1, nonempty="err"))
    cs.append(C("debug no dlv", ["debug", "fxplug"], fx="plugin", path="nodlv", rc=1, nonempty="err"))
    cs.append(C("debug plugin not found", ["debug", "nosuch"], rc=1, nonempty="err"))
    cs.append(C("debug full run", ["debug", "fxplug", "--port", "2399"], fx="plugin", rc=0, effect=1))
    cs.append(C("debug in plugin cwd", ["debug", "fxplug", "--port", "2398"], fx="incwd", rc=0, effect=1))
    cs.append(C("debug via link", ["debug", "fxplug", "--port", "2397"], fx="linked-plugin", cwd="sub", rc=0, effect=1))
    cs.append(C("debug link gone", ["debug", "ghost"], fx="linkedgone", rc=1, nonempty="err"))
    cs.append(C("debug build fails", ["debug", "fxplug", "--port", "2396"], fx="plugin", env={"STUB_FAIL_GO": "1"}, rc=2, nonempty="err"))
    cs.append(C("debug dlv sigint", ["debug", "fxplug", "--port", "2395"], fx="plugin", env={"STUB_DLV_RC": "130"}, rc=0))
    cs.append(C("debug dlv fails", ["debug", "fxplug", "--port", "2394"], fx="plugin", env={"STUB_DLV_RC": "3"}, rc=3, nonempty="err"))
    cs.append(C("debug no arg", ["debug"], rc=1, nonempty="err"))
    # ---- dev ----
    cs.append(C("dev not found", ["dev", "nosuch"], rc=1, nonempty="err"))
    cs.append(C("dev no-link", ["dev", "fxplug", "--no-link"], fx="plugin", rc=0, effect=1))
    cs.append(C("dev auto-link", ["dev", "fxplug"], fx="plugin", rc=0, effect=1))
    cs.append(C("dev in plugin dir", ["dev", "whatever"], fx="incwd", rc=0, effect=1))
    cs.append(C("dev via links", ["dev", "fxplug", "--no-link"], fx="linked-plugin", cwd="sub", rc=0, effect=1))
    cs.append(C("dev entrypoint ok", ["dev", "fxplug", "--no-link", "--entrypoint", "./cmd/x"], fx="plugin", rc=0, effect=1))
    cs.append(C("dev entrypoint traversal", ["dev", "fxplug", "--no-link", "--entrypoint", "../../etc"], fx="plugin", rc=1, nonempty="err"))
    cs.append(C("dev entrypoint dotdot clean", ["dev", "fxplug", "--no-link", "--entrypoint", "a/../.."], fx="plugin", rc=1, nonempty="err"))
    cs.append(C("dev entrypoint chars", ["dev", "fxplug", "--no-link", "--entrypoint", "a b;rm"], fx="plugin", rc=1, nonempty="err"))
    cs.append(C("dev autolink fails continues", ["dev", "fxplug"], fx="plugin-dotnselffile", rc=0, effect=1))
    cs.append(C("dev watch sigint", ["dev", "fxplug", "--no-link"], fx="plugin", env={"STUB_AIR_RC": "130"}, rc=0))
    cs.append(C("dev watch fails", ["dev", "fxplug", "--no-link"], fx="plugin", env={"STUB_AIR_RC": "3"}, rc=3, nonempty="err"))
    cs.append(C("dev debug delegates", ["dev", "fxplug", "--debug"], fx="plugin", rc=0, effect=1))
    cs.append(C("dev debug no dlv", ["dev", "fxplug", "--no-link", "--debug"], fx="plugin", path="nodlv", rc=1, nonempty="err"))
    cs.append(C("dev no arg", ["dev"], rc=1, nonempty="err"))
    # ---- test (unit phase; smoke phases call the registry in core and are covered by Go tests) ----
    cs.append(C("test bad phase", ["test", "fxplug", "--phase", "x"], fx="plugin", rc=1, nonempty="err"))
    cs.append(C("test not found", ["test", "nosuch", "--phase", "unit"], rc=1, nonempty="err"))
    cs.append(C("test unit host", ["test", "fxplug", "--phase", "unit", "--host"], fx="plugin", rc=0))
    cs.append(C("test unit docker", ["test", "fxplug", "--phase", "unit"], fx="plugin", rc=0))
    cs.append(C("test unit no docker", ["test", "fxplug", "--phase", "unit"], fx="plugin", env={"STUB_DOCKER_INFO_RC": "1"}, rc=0))
    cs.append(C("test unit fails docker", ["test", "fxplug", "--phase", "unit"], fx="plugin", env={"STUB_FAIL_GO": "1"}, rc=1, nonempty="err"))
    cs.append(C("test unit fails host", ["test", "fxplug", "--phase", "unit", "--host"], fx="plugin", env={"STUB_FAIL_GO": "1"}, rc=1, nonempty="err"))
    cs.append(C("test via cwd plugin", ["test", "any", "--phase", "unit", "--host"], fx="incwd", rc=0))
    cs.append(C("test no arg", ["test"], rc=1, nonempty="err"))
    # cases that must run a stub command: an empty command log would mean the case never reached the command
    for c in cs:
        if re.match(r"(debug full|debug in plugin|debug via|debug build|debug dlv|dev (no-link|auto-link|in plugin|via|entrypoint ok|autolink|watch|debug delegates)|test (unit|via))", c["id"]):
            c["runs"] = 1
    return cs


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
