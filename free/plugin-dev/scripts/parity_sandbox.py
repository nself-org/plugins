"""parity_sandbox.py: sandboxes, stubs, normalisation and one-side runner for parity_harness.py (P7-CANON-18).

Purpose: build an identical scripted sandbox (HOME, working directory, TMPDIR) for each side, run core
  `nself plugin <command>` or head `nself-plugin-dev <command>` in it, and return stdout, stderr, exit code,
  the recorded stub commands and a hash listing of the file effects.
Inputs:  configure(core_bin, head_bin, workdir) first; a case dict (see parity_cases.py) and a side name.
Outputs: run_side(case, side, root) -> dict(out, err, rc, cmds, effects), all normalised.
Constraints: no network; ~/.nself/logs (core's root command log) is not an effect of the command.
"""
import hashlib
import os
import re
import stat
import subprocess
import sys

CORE = HEAD = WORK = STUBS = None
SYSPATH = "/usr/bin:/bin"


def configure(core, head, work):
    global CORE, HEAD, WORK, STUBS
    CORE, HEAD, WORK = core, head, work
    STUBS = os.path.join(WORK, "stubs")

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


