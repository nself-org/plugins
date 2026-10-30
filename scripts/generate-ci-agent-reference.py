#!/usr/bin/env python3
"""Render the pull-agent reference from its Go flags and reason constants.

Inputs: pull.go and non-test transport/agent Go sources in this checkout.
Outputs: Agent-Reference.md, or a nonzero --check result when it is stale or absent.
Constraints: offline, stdlib only; fail if source declarations cannot be read.
"""

import argparse
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
PAGE = ROOT / ".github/wiki/guides/ci/Agent-Reference.md"


def render():
    """Build the scoped reference; source declarations own flags and reason keys."""
    agent = ROOT / "free/ci/internal/transport/agent"
    reasons = {}
    for path in sorted(agent.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        for name, value in re.findall(
            r'^\s*(?:const\s+)?(Reason\w+)\s*=\s*"([^"]+)"',
            path.read_text(), re.MULTILINE,
        ):
            reasons[name] = value
    pull = (ROOT / "free/ci/cmd/agent/pull.go").read_text()
    flags = re.findall(
        r'fs\.(?:Int|String|Bool)Var\(&f\.\w+, "([^"]+)", '
        r'([^,]+), "([^"]+)"\)', pull,
    )
    tokens = sorted(v for v in reasons.values() if not v.startswith("transport."))
    transport = sorted(v for v in reasons.values() if v.startswith("transport."))
    if not flags or not tokens or not transport:
        raise ValueError("cannot read pull flags or agent reason constants")
    lines = [
        "# CI agent reference", "",
        "<!-- Source: scripts/generate-ci-agent-reference.py; do not edit this page directly. -->", "",
        "This page covers the pull agent and its refusal and transport reasons. "
        "Commands, flags and codes of `nself ci` are on "
        "[[CI reference|guides/ci/Reference]].", "",
        "Regenerate with `python3 scripts/generate-ci-agent-reference.py`; "
        "validate with `python3 scripts/generate-ci-agent-reference.py --check`.", "",
        "## `nself-ci-agent pull`", "",
        "After `nself-ci-agent join`, keep an outbound TLS 1.3 mTLS connection "
        "to the enrolled coordinator and execute its leases. The enrollment pins "
        "the coordinator root; `--coordinator` selects its local record ID, not a URL.", "",
        "| Flag | Default | Meaning |", "| --- | --- | --- |",
    ]
    lines += [f"| `--{name}` | `{default}` | {description} |" for name, default, description in flags]
    lines += [
        "", "A slots value of zero keeps the automatic limit; a positive value lowers it. "
        "Negative values are rejected. Secret eligibility requires the project secret store "
        "on this node. `--dedicated` also permits release and deploy secrets.", "",
        "## E661: job token rejected", "",
        "The agent must present its own lease token using the certificate and session "
        "that first used it. Restart the job when its token expires or the attempt ends. "
        "Never copy a token between jobs, nodes, certificates or sessions.", "",
        "| Reason |", "| --- |",
    ]
    lines += [f"| `{reason}` |" for reason in tokens]
    lines += [
        "", "## E656: pull transport failure", "",
        "These keys describe the pull carrier. Check node connectivity and enrollment "
        "for connection/configuration failures, coordinator capacity for a full queue, "
        "and lease cancellation or duplication for lease failures. A blob refusal "
        "cancels the lease; inspect its integrity, size and sink verification.", "",
        "| Reason |", "| --- |",
    ]
    lines += [f"| `{reason}` |" for reason in transport]
    return "\n".join(lines) + "\n"


def main():
    """Write the reference or check it without changing any checkout files."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    expected = render()
    if args.check:
        if not PAGE.exists() or PAGE.read_text() != expected:
            print("CI agent reference is missing or stale; run "
                  "python3 scripts/generate-ci-agent-reference.py", file=sys.stderr)
            return 1
    else:
        PAGE.parent.mkdir(parents=True, exist_ok=True)
        PAGE.write_text(expected)
    return 0


if __name__ == "__main__":
    sys.exit(main())
