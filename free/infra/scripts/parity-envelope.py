#!/usr/bin/env python3
"""parity-envelope.py: compare one --json case of parity-server.sh (core legacy JSON vs head v1 envelope).

Purpose: since P7-SURF-29 the plugin prints one v1 envelope for --json and exits with the error class, while core
  prints its legacy JSON (or nothing on stdout when it fails). A byte diff of stdout can no longer be the check.
  This script checks the head's contract and the documented SURF-29 mapping instead.
Usage:   parity-envelope.py <sub> <core.out> <head.out> <core.rc> <head.rc>      (sub = provision|list|resize|destroy)
Checks:  1. head stdout is exactly one JSON document: a v1 envelope (schema_version "1", a command string, exactly one
            of data or error). Nothing before or after it.
         2. status: core exit 0 <=> envelope ok and head exit 0. Core exit 1 <=> error envelope whose exit_code equals
            the head exit, the exit_code matches its class (user 1, infra 2, auth 3, destructive_blocked 4, see
            cmd/errcodes.go) and is not 0.
         3. on success, core's JSON equals the envelope data under the SURF-29 mapping (key sets must match exactly,
            so a new or dropped field is a difference):
              provision  core object              == data.server        (data also holds user_data: bool)
              list       core array               == data.servers       (data also holds count == len)
              resize     core object              == data.action        (data also holds server_id, target_type)
              destroy    SnapshotID/RetainedIPs/ReleasedIPs == snapshot_id/retained_ips/released_ips (null == [];
                         data also holds server_id)
         The error text is not compared: core prints "Error: ..." on stderr, the head's envelope message is its own.
Output:  one line per problem; exit 0 when none, 1 otherwise.
"""
import json
import sys

CLASS_EXIT = {"user": 1, "infra": 2, "auth": 3, "destructive_blocked": 4}


def canon(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"))


def load_one(path):
    text = open(path, encoding="utf-8").read().lstrip()
    if not text:
        raise ValueError("stdout is empty, want exactly one envelope")
    doc, end = json.JSONDecoder().raw_decode(text)
    if text[end:].strip():
        raise ValueError("output after the envelope: %r" % text[end:end + 60])
    return doc


def check_envelope(doc):
    errs = []
    if not isinstance(doc, dict):
        return ["not a JSON object"]
    if doc.get("schema_version") != "1":
        errs.append("schema_version is %r, want '1'" % (doc.get("schema_version"),))
    if not isinstance(doc.get("command"), str) or not doc.get("command"):
        errs.append("command missing")
    if ("data" in doc) == ("error" in doc):
        errs.append("want exactly one of data or error")
    err = doc.get("error")
    if err is not None:
        if err.get("class") not in CLASS_EXIT or CLASS_EXIT[err.get("class")] != err.get("exit_code"):
            errs.append("exit_code %r does not match class %r" % (err.get("exit_code"), err.get("class")))
        if not isinstance(err.get("code"), str):
            errs.append("error.code missing")
    return errs


def need_keys(data, keys, errs):
    if not isinstance(data, dict) or set(data) != set(keys):
        errs.append("data keys %s, want %s" % (sorted(data) if isinstance(data, dict) else data, sorted(keys)))
        return False
    return True


def compare_data(sub, core, data):
    errs = []
    if sub == "provision":
        if need_keys(data, ["server", "user_data"], errs):
            if canon(core) != canon(data["server"]):
                errs.append("core server != data.server")
            if not isinstance(data["user_data"], bool):
                errs.append("data.user_data is not a boolean")
    elif sub == "list":
        if need_keys(data, ["servers", "count"], errs):
            if canon(core) != canon(data["servers"]):
                errs.append("core list != data.servers")
            if data["count"] != len(core):
                errs.append("data.count %r != %d" % (data["count"], len(core)))
    elif sub == "resize":
        if need_keys(data, ["server_id", "target_type", "action"], errs):
            if canon(core) != canon(data["action"]):
                errs.append("core action != data.action")
    elif sub == "destroy":
        if need_keys(data, ["server_id", "snapshot_id", "retained_ips", "released_ips"], errs):
            want = {"snapshot_id": core.get("SnapshotID"), "retained_ips": core.get("RetainedIPs") or [],
                    "released_ips": core.get("ReleasedIPs") or []}
            got = {k: (data[k] or [] if k.endswith("_ips") else data[k]) for k in want}
            if canon(want) != canon(got):
                errs.append("core destroy result != data: core=%s data=%s" % (canon(want), canon(got)))
    else:
        errs.append("no data mapping for subcommand %r" % sub)
    return errs


def main(argv):
    sub, core_out, head_out, core_rc, head_rc = argv[1], argv[2], argv[3], int(argv[4]), int(argv[5])
    try:
        doc = load_one(head_out)
    except ValueError as e:
        print("  head stdout: %s" % e)
        return 1
    errs = check_envelope(doc)
    if core_rc not in (0, 1):
        errs.append("core exit %d, want 0 or 1" % core_rc)
    ok = "error" not in doc
    if core_rc == 0:
        if not ok:
            errs.append("core succeeded (exit 0) but the head envelope is an error")
        if head_rc != 0:
            errs.append("core exit 0, head exit %d" % head_rc)
        if ok:
            try:
                errs += compare_data(sub, load_one(core_out), doc.get("data"))
            except ValueError as e:
                errs.append("core stdout: %s" % e)
    else:
        if ok:
            errs.append("core failed (exit %d) but the head envelope says ok" % core_rc)
        else:
            if head_rc == 0 or head_rc != doc["error"].get("exit_code"):
                errs.append("core exit %d, head exit %d, envelope exit_code %r" % (core_rc, head_rc, doc["error"].get("exit_code")))
    for e in errs:
        print("  " + e)
    return 1 if errs else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
