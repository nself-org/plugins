#!/usr/bin/env python3
"""Compare manifest service ports with literal/default compose targets and EXPOSE.

Inputs: plugin root and optional comma-separated names. Outputs: mismatches, exit 1.
Constraint: inspect declared compose fragments only; never execute interpolation.
"""
import json
import re
import sys
from pathlib import Path


def compose_ports(path, name):
    """Read published container targets from the primary service's ports block."""
    text = path.read_text()
    # Resolve only explicit numeric defaults, leaving unknown substitutions alone.
    text = re.sub(r"\$\{[A-Za-z_][A-Za-z_0-9]*:-?([0-9]+)\}", r"\1", text)
    services = {}
    service = None
    in_services = False
    in_ports = False
    declared = set()
    targets = []
    for line in text.splitlines():
        if line == "services:":
            in_services = True
            continue
        if not in_services:
            continue
        if line and not line[0].isspace() and not line.startswith("#"):
            break
        match = re.match(r"^  ([\w-]+):\s*(?:#.*)?$", line)
        if match:
            service = match[1]
            targets = services.setdefault(service, [])
            in_ports = False
            continue
        if service is None:
            continue
        if re.match(r"^    [\w-]+:", line):
            in_ports = bool(re.match(r"^    ports:", line))
            if in_ports:
                declared.add(service)
                inline = line.split("ports:", 1)[1].strip()
                if inline.startswith("["):
                    for item in inline[1:-1].split(","):
                        value = item.strip().strip("\"'").rsplit(":", 1)[-1].split("/", 1)[0]
                        if value.isdigit():
                            targets.append(int(value))
            continue
        if not in_ports:
            continue
        target = re.search(r"\btarget:\s*[\"']?(\d+)", line)
        if target:
            targets.append(int(target[1]))
            continue
        item = re.match(r"^\s+-\s+[\"']?([^\"'#]+)", line)
        if item:
            target = item[1].strip().rsplit(":", 1)[-1].split("/", 1)[0]
            if target.isdigit():
                targets.append(int(target))
    primary = next((key for key in (name, "plugin-" + name) if key in services), None)
    if primary is None and len(services) == 1:
        primary = next(iter(services))
    if primary is None:
        raise ValueError("primary service cannot be identified")
    if primary in declared and not services[primary]:
        raise ValueError("declared ports have no numeric container target")
    return services[primary]


def mismatches(root, selected):
    """Yield discrepancies for numeric service ports, preserving CLI-only plugins."""
    for manifest in sorted(root.glob("*/plugin.json")):
        data = json.loads(manifest.read_text())
        name = data.get("name", manifest.parent.name)
        if selected and name not in selected:
            continue
        service = data.get("service", {})
        port = service.get("port")
        if not isinstance(port, int) or not port:
            continue
        if data.get("port", port) != port:
            yield f"mismatch {name} manifest: service.port {port} vs port {data['port']}"
        compose = service.get("compose")
        if compose and (manifest.parent / compose).is_file():
            try:
                targets = compose_ports(manifest.parent / compose, name)
            except ValueError as error:
                yield f"mismatch {name} compose: {error}"
                targets = []
            if targets and port not in targets:
                yield f"mismatch {name} compose: service.port {port} vs targets {targets}"
        dockerfile = manifest.parent / "Dockerfile"
        if dockerfile.is_file():
            # Only the final build stage supplies the runtime image's EXPOSE.
            runtime = re.split(r"(?im)^FROM\s", dockerfile.read_text())[-1]
            exposed = [int(p) for raw in runtime.splitlines()
                       for line in [raw.split("#", 1)[0]]
                       if re.match(r"^\s*EXPOSE\s", line, re.I)
                       for p in re.findall(r"\b(\d+)(?:/(?:tcp|udp))?\b", line)]
            if exposed and port not in exposed:
                yield f"mismatch {name} Dockerfile: service.port {port} vs EXPOSE {exposed}"


if __name__ == "__main__":
    selected = set(filter(None, sys.argv[2].split(","))) if len(sys.argv) > 2 else set()
    errors = list(mismatches(Path(sys.argv[1]), selected))
    for error in errors:
        print(error)
    sys.exit(bool(errors))
