#!/usr/bin/env python3
"""Apply the post-P7-TRUST-21 Forgejo surface to a frozen core capture."""
import json
from pathlib import Path

root = Path(__file__).resolve().parent.parent
capture = root / "testdata/core-surface"
path = capture / "ci-forgejo.help.txt"
text = path.read_text()
text = ("Query the Forgejo server health endpoint. Workflow jobs run on external\n"
        "runners registered on a CI node that is not a deploy host. This plugin\n"
        "ships no runner container.\n" + text[text.index("\nEnv vars"):])
text = "\n".join(line for line in text.split("\n") if "--runner" not in line)
path.write_text(text)
(root / "internal/surface/help/ci-forgejo.txt").write_text(text)
path = capture / "help-json-ci.json"
data = json.loads(path.read_text())
for command in data["commands"]:
    if command["path"] == "nself ci forgejo":
        command["flags"] = [f for f in command["flags"] if f["name"] != "runner"]
        command["summary"] = "Show Forgejo server health (ops profile)"
path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")

path = capture / "ci.help.txt"
text = path.read_text().replace("Show Forgejo server + runner health", "Show Forgejo server health")
path.write_text(text)
(root / "internal/surface/help/ci.txt").write_text(text)
