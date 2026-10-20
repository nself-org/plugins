CREATE TABLE trust_run_facts (
 run TEXT PRIMARY KEY, author TEXT NOT NULL, head_repo TEXT NOT NULL,
 requires_approval INTEGER NOT NULL CHECK (requires_approval IN (0,1)), recorded_at INTEGER NOT NULL
);
CREATE TABLE trust_approvals (
 run TEXT PRIMARY KEY, head_sha TEXT NOT NULL, config_digest TEXT NOT NULL, head_repo TEXT NOT NULL,
 approver TEXT NOT NULL, spend INTEGER NOT NULL CHECK (spend IN (0,1)), approved_at INTEGER NOT NULL,
 voided_at INTEGER, void_reason TEXT NOT NULL DEFAULT ''
);
