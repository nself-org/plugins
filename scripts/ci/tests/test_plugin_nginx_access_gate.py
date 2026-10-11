"""Access gate on plugin nginx server blocks whose API has no auth (D-0616).

`nself build` copies every installed plugin's nginx/*.conf into the public
nginx. The cron and jobs services authenticate nobody, so their server blocks
must refuse every caller except loopback: `allow 127.0.0.1; allow ::1;
deny all;` at server level, inherited by every location. This test fails if a
listed conf loses that gate, widens it, or adds a directive that can answer or
redirect a request around it.

Runs under unittest (no third-party packages, no docker, no network) and
under pytest. The systemic sweep of the other plugins (D-0616) adds files to
GATED.
"""
import glob
import os
import unittest

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..")
# Every conf the CLI would inject for these plugins (it globs nginx/*.conf).
GATED_PLUGINS = ["cron", "jobs"]
LOOPBACK = {"127.0.0.1", "::1"}
# Directives that answer, redirect or re-route a request before or instead of
# the access phase, or that let a request through some other way.
# real_ip directives rewrite $remote_addr from a header, so a spoofed
# X-Forwarded-For: 127.0.0.1 would pass the loopback allow.
BYPASS = {"if", "rewrite", "break", "return", "include", "error_page",
          "access_by_lua", "access_by_lua_block", "access_by_lua_file",
          "set_real_ip_from", "real_ip_header", "real_ip_recursive"}


def tokens(text):
    """Split nginx config text into words, '{', '}' and ';', dropping comments."""
    out, word, i, n = [], "", 0, len(text)
    while i < n:
        c = text[i]
        if c == "\\":  # nginx keeps the escaped character literally
            word += text[i + 1:i + 2]
            i += 2
            continue
        if c in "\"'":  # quoted string, honouring \" escapes as nginx does
            j = i + 1
            while text[j] != c:
                j += 2 if text[j] == "\\" else 1
                if j >= n:
                    raise ValueError("unterminated quote")
            word += text[i + 1:j].replace("\\" + c, c)
            i = j + 1
            continue
        if c == "$" and text.startswith("{", i + 1):  # ${NAME} stays one word
            j = text.index("}", i)
            word += text[i:j + 1]
            i = j + 1
            continue
        if c == "#" and not word:
            while i < n and text[i] != "\n":
                i += 1
            continue
        if c.isspace() or c in "{};":
            if word:
                out.append(word)
                word = ""
            if c in "{};":
                out.append(c)
        else:
            word += c
        i += 1
    if word:
        out.append(word)
    return out


def parse(toks):
    """Nested directives: [(name, args, children or None)]."""
    def block(pos):
        items, cur = [], []
        while pos < len(toks):
            t = toks[pos]
            if t == ";":
                items.append((cur[0], cur[1:], None))
                cur = []
            elif t == "{":
                kids, pos = block(pos + 1)
                if pos >= len(toks):
                    raise ValueError("unterminated block: " + " ".join(cur))
                items.append((cur[0], cur[1:], kids))
                cur = []
            elif t == "}":
                return items, pos
            else:
                cur.append(t)
            pos += 1
        if cur:
            raise ValueError("unterminated directive: " + " ".join(cur))
        return items, pos
    items, pos = block(0)
    if pos < len(toks):
        raise ValueError("unbalanced '}'")
    return items


def walk(items):
    for name, args, kids in items:
        yield name, args, kids
        if kids is not None:
            yield from walk(kids)


def gate_problems(text):
    """Everything that keeps text from being a loopback-only server block."""
    try:
        tree = parse(tokens(text))
    except (ValueError, IndexError) as e:
        return ["does not parse: %s" % e]
    servers = [kids for name, _, kids in tree if name == "server" and kids is not None]
    if not servers:
        return ["no server block"]
    probs = []
    for kids in servers:
        top = [(n, a) for n, a, k in kids if k is None]
        if ("deny", ["all"]) not in top:
            probs.append("no server-level `deny all;`")
        for n, a in top:
            if n == "allow" and (len(a) != 1 or a[0] not in LOOPBACK):
                probs.append("server-level allow beyond loopback: allow %s" % " ".join(a))
            if n == "deny" and a != ["all"]:
                probs.append("unexpected deny %s" % " ".join(a))
        for n, a, k in walk(kids):
            if n == "satisfy" and a != ["all"]:
                probs.append("satisfy %s lets another check pass the request" % " ".join(a))
            if n in BYPASS:
                probs.append("`%s` can answer or redirect around the gate" % n)
        for n, _, k in kids:
            if k is None:
                continue
            for m, a, _ in walk(k):
                if m in ("allow", "deny"):
                    probs.append("%s block declares its own `%s %s`, which drops the "
                                 "inherited gate" % (n, m, " ".join(a)))
    return probs


GOOD = """server {
    listen 443 ssl; server_name x.${BASE_DOMAIN};
    allow 127.0.0.1;
    allow ::1;
    deny all;
    set $upstream http://svc:1;
    location / { proxy_pass $upstream; } # deny all in a comment is ignored
}
"""

# free/cron/nginx/cron.conf as plugins#166 (70899d5a) shipped it.
UNGATED_166 = """server {
    listen 443 ssl;
    http2 on;
    server_name cron.${BASE_DOMAIN};
    resolver 127.0.0.11 valid=10s;
    set $upstream http://plugin-cron:3051;
    location /v1/ {
        proxy_pass $upstream;
        client_max_body_size 1m;
    }
}
"""


class AccessGate(unittest.TestCase):
    def test_shipped_confs_are_gated(self):
        for plugin in GATED_PLUGINS:
            confs = glob.glob(os.path.join(ROOT, "free", plugin, "nginx", "*.conf"))
            self.assertTrue(confs, "free/%s/nginx has no .conf" % plugin)
            for path in confs:
                with self.subTest(conf=os.path.relpath(path, ROOT)):
                    with open(path) as fh:
                        self.assertEqual(gate_problems(fh.read()), [])

    def test_accepts_loopback_gate(self):
        self.assertEqual(gate_problems(GOOD), [])

    def test_rejects_the_166_conf(self):
        self.assertIn("no server-level `deny all;`", gate_problems(UNGATED_166))

    def test_rejects_widening_and_bypasses(self):
        cases = {
            "private range": ("allow ::1;", "allow ::1; allow 10.0.0.0/8;"),
            "docker range": ("allow ::1;", "allow ::1; allow 172.16.0.0/12;"),
            "allow all": ("allow ::1;", "allow all;"),
            "deny in comment only": ("deny all;", "# deny all;"),
            "location allow": ("proxy_pass $upstream;", "allow all; proxy_pass $upstream;"),
            "location deny ip": ("proxy_pass $upstream;", "deny 192.0.2.1; proxy_pass $upstream;"),
            "nested limit_except": ("proxy_pass $upstream;",
                                    "limit_except GET { allow all; } proxy_pass $upstream;"),
            "error_page": ("deny all;", "deny all; error_page 403 = @up;"),
            "return in location": ("proxy_pass $upstream;", "return 200;"),
            "if": ("proxy_pass $upstream;", "if ($arg_x) { set $a 1; } proxy_pass $upstream;"),
            "satisfy any": ("deny all;", "deny all; satisfy any;"),
            "include": ("deny all;", "deny all; include /etc/nginx/x.conf;"),
            "real_ip": ("deny all;", "deny all; set_real_ip_from 0.0.0.0/0; "
                                     "real_ip_header X-Forwarded-For;"),
            "escaped quote hides deny": ("deny all;",
                                         'set $n "a\\" ; deny all ; x \\"c";'),
        }
        texts = {name: GOOD.replace(old, new, 1) for name, (old, new) in cases.items()}
        texts["second ungated server"] = GOOD + UNGATED_166
        for name, text in texts.items():
            with self.subTest(case=name):
                self.assertNotEqual(text, GOOD)
                self.assertNotEqual(gate_problems(text), [])

    def test_rejects_unparseable(self):
        self.assertNotEqual(gate_problems("server { deny all;"), [])
        self.assertNotEqual(gate_problems("location / { deny all; }"), [])


if __name__ == "__main__":
    unittest.main()
