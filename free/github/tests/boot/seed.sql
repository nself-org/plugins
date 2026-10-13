-- Boot-kit seed (tests/boot/tables-upgrade.sh): one row per table the basis (v1.2.6) creates, written through the basis
-- schema before the upgrade. Test data only: fake ids and names.

INSERT INTO np_github_organizations (id, login) VALUES (9001, 'seed');
INSERT INTO np_github_repositories (id, name, full_name, owner_login) VALUES (9001, 'seed', 'seed', 'seed');
INSERT INTO np_github_branches (id, name, sha) VALUES ('seed-np_github_branches', 'seed', 'seed');
INSERT INTO np_github_issues (id, number, title) VALUES (9001, 1, 'seed');
INSERT INTO np_github_pull_requests (id, number, title) VALUES (9001, 1, 'seed');
INSERT INTO np_github_pr_reviews (id) VALUES (9001);
INSERT INTO np_github_issue_comments (id) VALUES (9001);
INSERT INTO np_github_pr_review_comments (id) VALUES (9001);
INSERT INTO np_github_commit_comments (id, commit_sha) VALUES (9001, 'seed');
INSERT INTO np_github_commits (sha) VALUES ('seed-np_github_commits');
INSERT INTO np_github_releases (id, tag_name) VALUES (9001, 'seed');
INSERT INTO np_github_tags (id, name, sha) VALUES ('seed-np_github_tags', 'seed', 'seed');
INSERT INTO np_github_milestones (id, number, title) VALUES (9001, 1, 'seed');
INSERT INTO np_github_labels (id, name) VALUES (9001, 'seed');
INSERT INTO np_github_workflows (id, name) VALUES (9001, 'seed');
INSERT INTO np_github_workflow_runs (id) VALUES (9001);
INSERT INTO np_github_workflow_jobs (id, name) VALUES (9001, 'seed');
INSERT INTO np_github_check_suites (id, head_sha) VALUES (9001, 'seed');
INSERT INTO np_github_check_runs (id, head_sha, name) VALUES (9001, 'seed', 'seed');
INSERT INTO np_github_deployments (id) VALUES (9001);
INSERT INTO np_github_teams (id, org_login, name, slug) VALUES (9001, 'seed', 'seed', 'seed');
INSERT INTO np_github_collaborators (id, login) VALUES (9001, 'seed');
INSERT INTO np_github_webhook_events (id, event) VALUES ('seed-np_github_webhook_events', 'seed');
