-- Expand-only. One row per (alert kind, resource) the coordinator has already told the operator about, so a restart does not repeat the alert (D-1330). Pruned with the decision log. Time is unix milliseconds.
CREATE TABLE burst_alert_seen (kind TEXT NOT NULL, resource TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(kind, resource)) WITHOUT ROWID;
CREATE INDEX burst_alert_seen_at ON burst_alert_seen(at);
