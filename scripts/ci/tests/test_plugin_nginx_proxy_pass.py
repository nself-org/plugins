"""proxy_pass lint for plugin nginx snippets (D-0600).

With a variable in proxy_pass, nginx sends the URI written in the directive
instead of the request URI. `proxy_pass http://$upstream_home/;` therefore
delivered every request to the plugin as `/`: path and query were lost. A
variable proxy_pass must carry no URI; strip a path prefix with `rewrite ...
break;` before it. This test also pins each `set $upstream_x host:port` to the
plugin's own compose service and container port, because 127.0.0.1 inside the
nginx container is nginx itself, not the plugin.

Runs under unittest (no third-party packages, no docker, no network) and
under pytest. Tokenizer and parser come from test_plugin_nginx_access_gate.

Known limit: literal `proxy_pass http://127.0.0.1:<port>/;` (file-processing,
media-processing) and `upstream { server 127.0.0.1:... }` blocks are not flagged;
they are tracked as debt (D-0616 follow-up).
"""
import glob
import os
import re
import unittest

from test_plugin_nginx_access_gate import ROOT, parse, tokens, walk

# `${NAME}` is a CLI template variable (rendered at build time), not an nginx one.
TEMPLATE_VAR = re.compile(r"\$\{[A-Za-z0-9_]+\}")
NGINX_VAR = re.compile(r"\$[A-Za-z_]")
SCHEME = re.compile(r"^[A-Za-z][A-Za-z0-9+.-]*://")
HOST_PORT = re.compile(r"^([A-Za-z0-9_.-]+):(\d+)$")
LOOPBACK = ("127.", "localhost", "::1")


def set_values(tree):
    """{$var: value} for every `set $var value;` in the tree."""
    return {a[0]: a[1] for n, a, _ in walk(tree) if n == "set" and len(a) == 2}


def expand(value, sets, depth=4):
    """Replace $vars that a `set` defines with their value (chained sets too)."""
    for _ in range(depth):
        new = re.sub(r"\$[A-Za-z_][A-Za-z0-9_]*",
                     lambda m: sets.get(m.group(0), m.group(0)), value)
        if new == value:
            break
        value = new
    return value


def proxy_pass_problems(text):
    """Variable proxy_pass directives that carry a URI, in the directive or in
    the `set` value the variable holds (`set $u home:3127/;` is the same bug)."""
    try:
        tree = parse(tokens(text))
    except (ValueError, IndexError) as e:
        return ["does not parse: %s" % e]
    sets = set_values(tree)
    probs = []
    for name, args, _ in walk(tree):
        if name != "proxy_pass" or len(args) != 1:
            continue
        target = TEMPLATE_VAR.sub("", args[0])
        if not NGINX_VAR.search(target):
            continue  # literal upstream: nginx replaces the matched prefix
        authority = SCHEME.sub("", expand(target, sets), count=1)
        if "/" in authority:
            probs.append("proxy_pass %s: a variable proxy_pass must carry no URI "
                         "(nginx sends that URI instead of the request's)" % args[0])
    return probs


def compose_services(text):
    """{service: container port} from a plugin docker-compose.plugin.yml."""
    out, svc, in_services = {}, None, False
    for line in text.splitlines():
        if line.lstrip().startswith("#"):
            continue
        if re.match(r"^services:\s*$", line):
            in_services = True
            continue
        if in_services and re.match(r"^\S", line) and not line.startswith("#"):
            break
        m = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
        if in_services and m:
            svc = m.group(1)
            continue
        m = re.search(r'"?127\.0\.0\.1:\d+:(\d+)"?', line)
        if svc and m and svc not in out:
            out[svc] = m.group(1)
    return out


def upstream_problems(text, services):
    """`set $upstream_x host:port` must name a compose service and its port; no
    `set` value that a proxy_pass uses may be a loopback address."""
    try:
        tree = parse(tokens(text))
    except (ValueError, IndexError) as e:
        return ["does not parse: %s" % e]
    used = {m for n, a, _ in walk(tree) if n == "proxy_pass" for m in re.findall(r"\$\w+", " ".join(a))}
    probs = []
    for name, args, _ in walk(tree):
        if name != "set" or len(args) != 2:
            continue
        if not (args[0].startswith("$upstream") or args[0] in used):
            continue
        m = HOST_PORT.match(SCHEME.sub("", args[1], count=1).rstrip("/"))
        if not m:
            continue
        host, port = m.groups()
        if host.startswith(LOOPBACK):
            probs.append("set %s %s: loopback inside the nginx container is nginx, "
                         "not the plugin; use the compose service name" % (args[0], args[1]))
        elif args[0].startswith("$upstream") and host not in services:
            probs.append("set %s %s: no compose service named %s (have %s)"
                         % (args[0], args[1], host, ", ".join(sorted(services)) or "none"))
        elif args[0].startswith("$upstream") and services[host] != port:
            probs.append("set %s %s: compose publishes %s on %s"
                         % (args[0], args[1], host, services[host]))
    return probs


# free/home/nginx/home.conf as shipped before D-0600.
BROKEN_HOME = """location /plugins/home/ {
    set $upstream_home 127.0.0.1:3127;
    proxy_pass http://$upstream_home/;
    proxy_set_header Host $host;
}
"""
FIXED_HOME = """location /plugins/home/ {
    resolver 127.0.0.11 valid=10s;
    set $upstream_home home:3127;
    rewrite ^/plugins/home/(.*)$ /$1 break;
    proxy_pass http://$upstream_home;
}
"""
HOME_SERVICES = {"home": "3127"}


class ProxyPassLint(unittest.TestCase):
    def test_shipped_confs_pass(self):
        confs = sorted(glob.glob(os.path.join(ROOT, "free", "*", "nginx", "*.conf")))
        self.assertGreater(len(confs), 50)  # the glob reads the tree, not nothing
        for path in confs:
            plugin_dir = os.path.dirname(os.path.dirname(path))
            compose = os.path.join(plugin_dir, "docker-compose.plugin.yml")
            services = {}
            if os.path.exists(compose):
                with open(compose) as fh:
                    services = compose_services(fh.read())
            with self.subTest(conf=os.path.relpath(path, ROOT)):
                with open(path) as fh:
                    text = fh.read()
                self.assertEqual(proxy_pass_problems(text), [])
                self.assertEqual(upstream_problems(text, services), [])

    def test_the_variable_snippets_strip_the_prefix(self):
        plugins = ["documents", "entitlements", "game-metadata", "home", "meetings",
                   "observability", "podcast", "post", "retro-gaming", "sports",
                   "tmdb", "web3", "workflows"]
        for p in plugins:
            with self.subTest(plugin=p):
                with open(os.path.join(ROOT, "free", p, "nginx", p + ".conf")) as fh:
                    tree = parse(tokens(fh.read()))
                locs = [k for n, a, k in tree if n == "location" and k is not None]
                self.assertEqual(len(locs), 1)
                kids = [(n, a) for n, a, _ in locs[0]]
                self.assertIn(("rewrite", ["^/plugins/%s/(.*)$" % p, "/$1", "break"]), kids)
                self.assertTrue(any(n == "resolver" for n, _ in kids))

    def test_commented_rewrite_does_not_count(self):
        tree = parse(tokens("location /p/ { # rewrite ^/p/(.*)$ /$1 break;\n proxy_pass http://$u; }"))
        self.assertNotIn("rewrite", [n for n, _, _ in tree[0][2]])

    def test_positive_control_broken_snippet_is_flagged(self):
        self.assertTrue(proxy_pass_problems(BROKEN_HOME))
        self.assertTrue(upstream_problems(BROKEN_HOME, HOME_SERVICES))

    def test_fixed_snippet_passes(self):
        self.assertEqual(proxy_pass_problems(FIXED_HOME), [])
        self.assertEqual(upstream_problems(FIXED_HOME, HOME_SERVICES), [])

    def test_uri_shapes(self):
        flagged = [
            "proxy_pass http://$u/;", "proxy_pass http://$u/api;", "proxy_pass http://$u/v1/;",
            "proxy_pass http://host:$port/;", "proxy_pass $u/;", "proxy_pass http://$u/x$request_uri;",
        ]
        clean = [
            "proxy_pass http://$u;", "proxy_pass $u;", "proxy_pass http://up;",
            "proxy_pass http://up/api/;", "proxy_pass http://host:3000/;",
            "proxy_pass http://${BASE_DOMAIN}/;",
        ]
        for line in flagged:
            with self.subTest(line=line):
                self.assertTrue(proxy_pass_problems("location / { %s }" % line))
        for line in clean:
            with self.subTest(line=line):
                self.assertEqual(proxy_pass_problems("location / { %s }" % line), [])

    def test_uri_hidden_in_a_set_value_is_flagged(self):
        for body in ("set $u home:3127/; proxy_pass http://$u;",
                     "set $u http://$a/; proxy_pass $u;",
                     "set $a home:3127/x; set $u http://$a; proxy_pass $u;"):
            with self.subTest(body=body):
                self.assertTrue(proxy_pass_problems("location / { %s }" % body))
        self.assertEqual(proxy_pass_problems("location / { set $u home:3127; proxy_pass http://$u; }"), [])

    def test_loopback_in_any_used_set_is_flagged(self):
        self.assertTrue(upstream_problems("location / { set $backend 127.0.0.1:3127; proxy_pass http://$backend; }", {}))
        self.assertEqual(upstream_problems("location / { set $unused 127.0.0.1:3127; }", {}), [])

    def test_upstream_must_match_compose(self):
        loc = "location / { set $upstream_x %s; }"
        self.assertEqual(upstream_problems(loc % "home:3127", HOME_SERVICES), [])
        for bad in ("home:3128", "other:3127", "localhost:3127", "127.0.0.1:3127", "http://127.0.0.1:3127"):
            with self.subTest(upstream=bad):
                self.assertTrue(upstream_problems(loc % bad, HOME_SERVICES))

    def test_compose_parser(self):
        text = ('services:\n  plugin-cron:\n    ports:\n      - "127.0.0.1:3051:3051"\n'
                '  sidecar:\n    image: x\nvolumes:\n  data:\n')
        self.assertEqual(compose_services(text), {"plugin-cron": "3051"})


if __name__ == "__main__":
    unittest.main()
