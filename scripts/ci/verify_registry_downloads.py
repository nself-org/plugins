#!/usr/bin/env python3
"""Verify free plugin checksums against the bytes their public download URLs serve.

Inputs: a local registry path or public HTTPS registry URL.
Outputs: per-plugin results and a nonzero exit on invalid metadata/download/hash.
Constraints: read-only, no credentials, no rebuilt archives or paid downloads.
"""

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from urllib.parse import urlparse, urlunparse
from urllib.request import Request, urlopen


def open_public(url):
    """Open a public HTTPS URL without authentication, following CDN redirects."""
    if urlparse(url).scheme != "https":
        raise ValueError("download URL must use HTTPS")
    request = Request(url, headers={"User-Agent": "nself-registry-check/1.0"})
    return urlopen(request, timeout=60)


def read_registry(source):
    """Load local candidate metadata or the public registry served to clients."""
    if source.startswith("https://"):
        with open_public(source) as response:
            return json.load(response)
    return json.loads(Path(source).read_text())


def plugin_rows(registry):
    """Normalize the repository's keyed and the worker's array wire formats."""
    plugins = registry.get("plugins")
    if isinstance(plugins, dict):
        return [dict(row, name=name) for name, row in plugins.items()]
    if isinstance(plugins, list):
        return plugins
    raise ValueError("registry plugins must be an object or array")


def verify_plugin(row, opener):
    """Hash served bytes and check every recorded source checksum alias."""
    name = row.get("name")
    if not isinstance(name, str) or not name:
        raise ValueError("plugin has no name")
    checksums = [row.get("checksum"), row.get("checksums", {}).get("sha256")]
    checksums = [value for value in checksums if value is not None]
    if not checksums or any(not isinstance(value, str) or
                            not re.fullmatch(r"[0-9a-fA-F]{64}", value)
                            for value in checksums):
        raise ValueError("missing or malformed SHA256")
    url = (row.get("download_url") or row.get("tarballUrl") or
           row.get("tarballURL") or row.get("tarball"))
    if not isinstance(url, str) or urlparse(url).scheme != "https":
        raise ValueError("missing public HTTPS download URL")
    digest = hashlib.sha256()
    with opener(url) as response:
        while chunk := response.read(1024 * 1024):
            digest.update(chunk)
        served_url = urlunparse(urlparse(response.geturl())._replace(query="", fragment=""))
    actual = digest.hexdigest()
    if any(actual != expected.lower() for expected in checksums):
        raise ValueError(f"SHA256 mismatch: recorded={','.join(checksums)} "
                         f"served={actual} url={served_url}")
    return actual, served_url


def verify(registry, opener=open_public, output=print):
    """Check all installable free entries, continuing after each failure."""
    checked = errors = 0
    seen = set()
    for row in plugin_rows(registry):
        if row.get("tier", "free") != "free" or row.get("requires_license") is True:
            continue
        if row.get("installable") is False:
            continue
        name = row.get("name", "<unnamed>")
        try:
            if name in seen:
                raise ValueError("duplicate plugin name")
            seen.add(name)
            actual, url = verify_plugin(row, opener)
            checked += 1
            output(f"OK {name}: sha256={actual} url={url}")
        except Exception as error:
            errors += 1
            output(f"ERROR {name}: {error}")
    if not checked and not errors:
        errors += 1
        output("ERROR: no installable free plugins were checked")
    output(f"Verified {checked} free plugin download(s); errors={errors}")
    return 1 if errors else 0


def main():
    """CLI entry point; unexpected registry/download errors fail closed."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registry", default="https://plugins.nself.org/registry.json")
    args = parser.parse_args()
    try:
        return verify(read_registry(args.registry))
    except Exception as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
