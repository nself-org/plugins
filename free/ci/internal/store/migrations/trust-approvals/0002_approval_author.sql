ALTER TABLE trust_approvals ADD COLUMN author TEXT NOT NULL DEFAULT '';
UPDATE trust_approvals SET author=coalesce((SELECT f.author FROM trust_run_facts f WHERE f.run=trust_approvals.run),'');
