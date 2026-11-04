-- Upgrade-proof seed (D-0832): one row per public table the basis Go DDL made, and one per np_cron copy the basis
-- migrations made (listed in extra-tables), so the proof shows the converted boot keeps both. Parents first.
INSERT INTO public.np_cron_jobs (id, name, cron_expr, callback_url, enabled) VALUES ('00000000-0000-4000-8000-0000000000a1', 'd0832-seed', '0 0 1 1 *', 'http://127.0.0.1:9/seed', false);
INSERT INTO public.np_cron_runs (id, job_id, status) VALUES ('00000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', 'success');
INSERT INTO np_cron.np_cron_jobs (id, name, cron_expr, callback_url, enabled) VALUES ('00000000-0000-4000-8000-0000000000a1', 'd0832-seed', '0 0 1 1 *', 'http://127.0.0.1:9/seed', false);
INSERT INTO np_cron.np_cron_runs (id, job_id, status) VALUES ('00000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', 'success');
