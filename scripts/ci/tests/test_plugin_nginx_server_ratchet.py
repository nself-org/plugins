"""Ratchet: no new ungated plugin nginx server block (D-0616).

`nself build` copies every installed plugin's nginx/*.conf into the public
nginx, so a conf that holds a `server {}` block publishes the plugin on its own
hostname. cron and jobs now refuse every caller but loopback (see
test_plugin_nginx_access_gate.py). UNGATED lists the server-block confs that
shipped without any gate before that fix: debt, not approval. Rules:

  * a server-block conf that is neither loopback-gated nor listed fails, so a
    new conf (or a D-0145 conversion) must add the gate first;
  * a listed conf that became gated, or no longer exists, fails until its line
    is deleted, so the list only shrinks.

Runs under unittest (no third-party packages, no docker, no network) and
under pytest.

Scope: the CLI injects `nginx/*.conf` only (a `.conf.tmpl` is never copied), and
this test reads top-level `server` blocks. Bare `location` snippets are out of
scope here: the CLI refuses them today (pluginSnippetSkip); see the D-0616
follow-up before that changes.
"""
import glob
import hashlib
import os
import unittest

from test_plugin_nginx_access_gate import ROOT, gate_problems, parse, tokens

# <plugin>/<file>.conf under free/ -> first 16 hex of the sha256 of its tokens
# (comments and spacing ignored). An edit to a listed conf fails until it is
# gated: the list pins today's routes, it does not license new ones. Remove a
# line when its conf is gated.
UNGATED = {
    "access-controls/access-controls.conf": "5b2d205ce26743a5",
    "admin-api/admin-api.conf": "5e8d1e466260e47b",
    "analytics/analytics.conf": "04ed847e0b7a8bed",
    "audit-log/audit-log.conf": "b79ecae290f34d8e",
    "backup/backup.conf": "fb1a89a77ff3bbfd",
    "cdn/cdn.conf": "f92d98470ff5cc2c",
    "cloudflare/cloudflare.conf": "4832c0f55e622dae",
    "compliance/compliance.conf": "22f852246f11009d",
    "content-acquisition/content-acquisition.conf": "a3a7d16af889b0f7",
    "content-progress/content-progress.conf": "729594883e9de187",
    "ddns/ddns.conf": "7fb555d6165367e0",
    "devices/devices.conf": "afb7ae23fb7c8777",
    "donorbox/donorbox.conf": "1518c4342b8b43bd",
    "feature-flags/feature-flags.conf": "34d11f5e3edfecf8",
    "geocoding/geocoding.conf": "d056ff3be6269543",
    "github-runner/github-runner.conf": "6ec018b7aad70713",
    "github/github.conf": "e28a1a650167742a",
    "idme/idme.conf": "d9b37ba98155bfa7",
    "invitations/invitations.conf": "33d226549c27f409",
    "link-preview/link-preview.conf": "aaf59b2400c15607",
    "mdns/mdns.conf": "a77ae552343d881a",
    "mlflow/mlflow.conf": "aa60b958cbe9867e",
    "monitoring/alertmanager.conf": "c259d1ea94788af9",
    "monitoring/grafana.conf": "c46f1f27ebd56d7e",
    "monitoring/prometheus.conf": "3ff30b6705ed14dd",
    "notifications/notifications.conf": "fdcb64a4954ba296",
    "notify/notify.conf": "769fd4fe71ed19ee",
    "object-storage/object-storage.conf": "021ac6384085fa10",
    "paypal/paypal.conf": "e2ae87a67617f396",
    "rom-discovery/rom-discovery.conf": "1ed3a0c23b67f964",
    "search/search.conf": "a31a72ed4c0170b3",
    "shopify/shopify.conf": "4a4c70b1e9d7bb09",
    "stripe/stripe.conf": "52b71be315c2b91e",
    "subtitle-manager/subtitle-manager.conf": "f7a04e13ab365ef4",
    "tokens/tokens.conf": "c6722b6a0e7f44a0",
    "torrent-manager/torrent-manager.conf": "beca8b100d039f7a",
    "vpn/vpn.conf": "6ac6c064e16532fc",
    "webhooks/webhooks.conf": "3b9c5d69fe1947ef",
}


def server_confs():
    """{<plugin>/<file>.conf: text} for every free plugin conf with a server block."""
    out = {}
    for path in sorted(glob.glob(os.path.join(ROOT, "free", "*", "nginx", "*.conf"))):
        with open(path) as fh:
            text = fh.read()
        if any(name == "server" for name, _, _ in parse(tokens(text))):
            out[os.path.relpath(path, os.path.join(ROOT, "free")).replace("/nginx/", "/")] = text
    return out


def digest(text):
    return hashlib.sha256(" ".join(tokens(text)).encode()).hexdigest()[:16]


def ratchet_problems(confs, ungated):
    probs = []
    for name, text in confs.items():
        gated = gate_problems(text) == []
        if not gated and name in ungated and ungated[name] != digest(text):
            probs.append("%s: a listed ungated conf changed; gate it instead of editing it "
                         "(allow 127.0.0.1; allow ::1; deny all;)" % name)
        elif not gated and name not in ungated:
            probs.append("%s: server block without the loopback gate; add the gate "
                         "(allow 127.0.0.1; allow ::1; deny all;) or an auth gate" % name)
        if gated and name in ungated:
            probs.append("%s: now gated; delete it from UNGATED" % name)
    for name in ungated:
        if name not in confs:
            probs.append("%s: listed in UNGATED but has no server block; delete it" % name)
    return probs


GATED_TEXT = "server { listen 443; deny all; allow 127.0.0.1; allow ::1; }"
OPEN_TEXT = "server { listen 443; server_name x; }"


class Ratchet(unittest.TestCase):
    def test_shipped_tree(self):
        confs = server_confs()
        self.assertGreater(len(confs), 30)  # the glob reads the tree, not nothing
        self.assertEqual(ratchet_problems(confs, UNGATED), [])

    def test_baseline_is_sorted_and_unique(self):
        self.assertEqual(list(UNGATED), sorted(UNGATED))

    def test_new_ungated_conf_fails(self):
        self.assertTrue(ratchet_problems({"new/new.conf": OPEN_TEXT}, UNGATED))

    def test_gated_conf_passes_and_listed_open_conf_passes(self):
        self.assertEqual(ratchet_problems({"new/new.conf": GATED_TEXT}, {}), [])
        self.assertEqual(ratchet_problems({"old/old.conf": OPEN_TEXT}, {"old/old.conf": digest(OPEN_TEXT)}), [])

    def test_stale_baseline_entries_fail(self):
        self.assertTrue(ratchet_problems({"old/old.conf": GATED_TEXT}, {"old/old.conf": "x"}))
        self.assertTrue(ratchet_problems({}, {"gone/gone.conf": "x"}))

    def test_editing_a_listed_conf_fails(self):
        listed = {"old/old.conf": digest(OPEN_TEXT)}
        grown = OPEN_TEXT.replace("}", "location /new/ { proxy_pass http://up; } }")
        self.assertTrue(ratchet_problems({"old/old.conf": grown}, listed))
        spaced = "# comment\n" + OPEN_TEXT.replace(" ", "  ")
        self.assertEqual(ratchet_problems({"old/old.conf": spaced}, listed), [])


if __name__ == "__main__":
    unittest.main()
