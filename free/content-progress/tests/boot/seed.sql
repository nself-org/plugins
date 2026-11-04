-- Upgrade-proof seed (D-0832): one row per public table the basis Go DDL made, and one per np_content_progress copy
-- the basis migrations made (listed in extra-tables), so the proof shows the converted boot keeps both.
INSERT INTO public.np_progress_positions (id, user_id, content_type, content_id, position_seconds) VALUES ('00000000-0000-4000-8000-0000000000b1', 'd0832-user', 'movie', 'm1', 42);
INSERT INTO public.np_progress_history (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b2', 'd0832-user', 'movie', 'm1');
INSERT INTO public.np_progress_watchlists (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b3', 'd0832-user', 'movie', 'm2');
INSERT INTO public.np_progress_favorites (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b4', 'd0832-user', 'movie', 'm3');
INSERT INTO public.np_progress_webhook_events (id, event_type) VALUES ('d0832-evt-1', 'progress.updated');
INSERT INTO np_content_progress.np_progress_positions (id, user_id, content_type, content_id, position_seconds) VALUES ('00000000-0000-4000-8000-0000000000b1', 'd0832-user', 'movie', 'm1', 42);
INSERT INTO np_content_progress.np_progress_history (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b2', 'd0832-user', 'movie', 'm1');
INSERT INTO np_content_progress.np_progress_watchlists (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b3', 'd0832-user', 'movie', 'm2');
INSERT INTO np_content_progress.np_progress_favorites (id, user_id, content_type, content_id) VALUES ('00000000-0000-4000-8000-0000000000b4', 'd0832-user', 'movie', 'm3');
INSERT INTO np_content_progress.np_progress_webhook_events (id, event_type) VALUES ('d0832-evt-1', 'progress.updated');
