"""parity_cases.py: the scripted case matrix for parity_harness.py (P7-CANON-18).

Purpose: one dict per case: id, argv (core spelling after `plugin`; the head binary gets the same argv),
  fixture name, and expectations (rc, effect, runs, nonempty, sorted, pty, env, path, stdin, cwd).
Inputs:  none. Outputs: cases() -> list of dicts.
Constraints: every case is offline; smoke install/uninstall phases of `test` are not run against core.
"""
import re


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


