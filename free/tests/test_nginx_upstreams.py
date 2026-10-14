"""D-0776: reject container-local proxy destinations in all free nginx confs.

Run: python3 -m unittest discover -s free/tests -p 'test_nginx_upstreams.py'
Reuses the CI nginx parser; no Docker, network, or third-party dependencies.
"""
import ipaddress
from pathlib import Path
import re
import sys
import unittest
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/ci/tests"))
from test_plugin_nginx_access_gate import parse, tokens, walk
from test_plugin_nginx_proxy_pass import expand, set_values


def loopback_problems(text):
    """Lint proxy_pass (including set chains) and upstream server addresses."""
    tree = parse(tokens(text))
    sets = set_values(tree)
    problems = []
    upstream_servers = [
        (name, args, children)
        for directive, _, block in walk(tree)
        if directive == "upstream" and block is not None
        for name, args, children in block if name == "server"
    ]
    proxies = [(n, a, k) for n, a, k in walk(tree) if n == "proxy_pass"]
    for name, args, _ in proxies + upstream_servers:
        target = expand(args[0], sets)
        host = urlsplit(target if "://" in target else "//" + target).hostname
        if not host:
            continue
        try:
            address = ipaddress.ip_address(host)
            loopback = address.is_loopback
            if address.version == 6 and address.ipv4_mapped:
                loopback = loopback or address.ipv4_mapped.is_loopback
        except ValueError:
            loopback = host.rstrip(".").lower() == "localhost"
        if loopback:
            problems.append(f"{name} {args[0]}: loopback reaches nginx itself")
    return problems


class NginxUpstreams(unittest.TestCase):
    def test_no_shipped_loopback_destinations(self):
        confs = sorted((ROOT / "free").glob("*/nginx/*.conf"))
        self.assertGreater(len(confs), 50)
        for conf in confs:
            with self.subTest(conf=str(conf.relative_to(ROOT))):
                self.assertEqual(loopback_problems(conf.read_text()), [])

    def test_loopback_forms_are_rejected(self):
        for target in ("127.0.0.1:80", "127.8.9.10:80", "localhost:80",
                       "LOCALHOST.:80", "[::1]:80", "[::ffff:127.0.0.1]:80"):
            for snippet in (f"location /p/ {{ proxy_pass http://{target}/; }}",
                            f"upstream p {{ server {target} weight=2; }}",
                            f"set $a {target}; set $b $a; proxy_pass http://$b;"):
                with self.subTest(snippet=snippet):
                    self.assertTrue(loopback_problems(snippet))

    def test_non_destination_loopbacks_are_allowed(self):
        text = """# proxy_pass http://127.0.0.1:80/;
        upstream p { server plugin:80; }
        location /p/ {
            resolver 127.0.0.11 valid=10s;
            allow 127.0.0.1;
            allow ::1;
            proxy_set_header Host localhost;
            proxy_pass http://p/;
        }"""
        self.assertEqual(loopback_problems(text), [])

    def test_affected_targets_match_compose_container_ports(self):
        plugins = ("access-controls admin-api analytics cdn cloudflare compliance "
                   "ddns devices geocoding idme object-storage file-processing "
                   "media-processing rom-discovery shopify").split()
        for plugin in plugins:
            directory = ROOT / "free" / plugin
            compose = (directory / "docker-compose.plugin.yml").read_text()
            service = re.search(r"^  ([\w-]+):\s*$", compose, re.MULTILINE)[1]
            port = re.search(r'^\s+- [\"\']?(?:127\.0\.0\.1:)?\d+:(\d+)',
                             compose, re.MULTILINE)[1]
            for conf in (directory / "nginx").glob("*.conf"):
                tree = parse(tokens(conf.read_text()))
                destinations = [a[0] for n, a, _ in walk(tree) if n == "server" and a]
                destinations += [a[1] for n, a, _ in walk(tree) if n == "set"]
                with self.subTest(plugin=plugin):
                    self.assertTrue(destinations)
                    self.assertEqual(set(destinations), {f"{service}:{port}"})

    def test_processing_snippets_preserve_paths(self):
        for plugin in ("file-processing", "media-processing"):
            conf = ROOT / "free" / plugin / "nginx" / f"{plugin}.conf"
            tree = parse(tokens(conf.read_text()))
            directives = [(n, a) for n, a, _ in tree[0][2]]
            with self.subTest(plugin=plugin):
                self.assertIn(("resolver", ["127.0.0.11", "valid=10s"]), directives)
                self.assertIn(("rewrite", [f"^/plugins/{plugin}/(.*)$", "/$1", "break"]),
                              directives)
                variable = "$" + plugin.replace("-", "_") + "_backend"
                self.assertIn(("proxy_pass", [f"http://{variable}"]), directives)


if __name__ == "__main__":
    unittest.main()
