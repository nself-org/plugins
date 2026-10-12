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
"""
import glob
import os
import unittest

from test_plugin_nginx_access_gate import ROOT, gate_problems, parse, tokens

# Paths under free/, as <plugin>/<file>.conf. Remove a line when its conf is gated.
UNGATED = [
    "access-controls/access-controls.conf",
    "admin-api/admin-api.conf",
    "analytics/analytics.conf",
    "audit-log/audit-log.conf",
    "backup/backup.conf",
    "cdn/cdn.conf",
    "cloudflare/cloudflare.conf",
    "compliance/compliance.conf",
    "content-acquisition/content-acquisition.conf",
    "content-progress/content-progress.conf",
    "ddns/ddns.conf",
    "devices/devices.conf",
    "donorbox/donorbox.conf",
    "feature-flags/feature-flags.conf",
    "geocoding/geocoding.conf",
    "github-runner/github-runner.conf",
    "github/github.conf",
    "idme/idme.conf",
    "invitations/invitations.conf",
    "link-preview/link-preview.conf",
    "mdns/mdns.conf",
    "mlflow/mlflow.conf",
    "monitoring/alertmanager.conf",
    "monitoring/grafana.conf",
    "monitoring/prometheus.conf",
    "notifications/notifications.conf",
    "notify/notify.conf",
    "object-storage/object-storage.conf",
    "paypal/paypal.conf",
    "rom-discovery/rom-discovery.conf",
    "search/search.conf",
    "shopify/shopify.conf",
    "stripe/stripe.conf",
    "subtitle-manager/subtitle-manager.conf",
    "tokens/tokens.conf",
    "torrent-manager/torrent-manager.conf",
    "vpn/vpn.conf",
    "webhooks/webhooks.conf",
]


def server_confs():
    """{<plugin>/<file>.conf: text} for every free plugin conf with a server block."""
    out = {}
    for path in sorted(glob.glob(os.path.join(ROOT, "free", "*", "nginx", "*.conf"))):
        with open(path) as fh:
            text = fh.read()
        if any(name == "server" for name, _, _ in parse(tokens(text))):
            out[os.path.relpath(path, os.path.join(ROOT, "free")).replace("/nginx/", "/")] = text
    return out


def ratchet_problems(confs, ungated):
    probs = []
    for name, text in confs.items():
        gated = gate_problems(text) == []
        if not gated and name not in ungated:
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
        self.assertEqual(UNGATED, sorted(set(UNGATED)))

    def test_new_ungated_conf_fails(self):
        self.assertTrue(ratchet_problems({"new/new.conf": OPEN_TEXT}, UNGATED))

    def test_gated_conf_passes_and_listed_open_conf_passes(self):
        self.assertEqual(ratchet_problems({"new/new.conf": GATED_TEXT}, []), [])
        self.assertEqual(ratchet_problems({"old/old.conf": OPEN_TEXT}, ["old/old.conf"]), [])

    def test_stale_baseline_entries_fail(self):
        self.assertTrue(ratchet_problems({"old/old.conf": GATED_TEXT}, ["old/old.conf"]))
        self.assertTrue(ratchet_problems({}, ["gone/gone.conf"]))


if __name__ == "__main__":
    unittest.main()
