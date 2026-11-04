-- Upgrade-proof seed (D-0832): one row per public table the basis Go DDL made, and one per np_compliance copy the
-- basis migrations made (listed in extra-tables), so the proof shows the converted boot keeps both. Parents first.
INSERT INTO public.np_change_log (id, change_type, description) VALUES ('00000000-0000-4000-8000-0000000000c1', 'config', 'd0832 seed');
INSERT INTO public.np_access_reviews (id, period_start, period_end) VALUES ('00000000-0000-4000-8000-0000000000c2', '2026-01-01', '2026-03-31');
INSERT INTO public.np_vendor_registry (id, vendor_name) VALUES ('00000000-0000-4000-8000-0000000000c3', 'd0832-vendor');
INSERT INTO np_compliance.np_change_log (id, change_type, description) VALUES ('00000000-0000-4000-8000-0000000000c1', 'config', 'd0832 seed');
INSERT INTO np_compliance.np_access_reviews (id, period_start, period_end) VALUES ('00000000-0000-4000-8000-0000000000c2', '2026-01-01', '2026-03-31');
INSERT INTO np_compliance.np_vendor_registry (id, vendor_name) VALUES ('00000000-0000-4000-8000-0000000000c3', 'd0832-vendor');
INSERT INTO public.compliance_dsars (id, request_type, request_number, requester_email, deadline) VALUES ('00000000-0000-4000-8000-0000000000d1', 'access', 'DSAR-D0832', 'seed@example.invalid', '2026-12-31');
INSERT INTO public.compliance_dsar_activities (id, dsar_id, activity_type) VALUES ('00000000-0000-4000-8000-0000000000d2', '00000000-0000-4000-8000-0000000000d1', 'created');
INSERT INTO public.compliance_consents (id, user_id, purpose) VALUES ('00000000-0000-4000-8000-0000000000d3', 'd0832-user', 'marketing');
INSERT INTO public.compliance_consent_history (id, consent_id, new_status) VALUES ('00000000-0000-4000-8000-0000000000d4', '00000000-0000-4000-8000-0000000000d3', 'granted');
INSERT INTO public.compliance_privacy_policies (id, version, version_number, title, content, effective_from) VALUES ('00000000-0000-4000-8000-0000000000d5', 'v1', 1, 'Policy', 'Text', '2026-01-01');
INSERT INTO public.compliance_data_exports (id, user_id) VALUES ('00000000-0000-4000-8000-0000000000d6', 'd0832-user');
INSERT INTO public.compliance_webhook_events (id, event_type) VALUES ('00000000-0000-4000-8000-0000000000d7', 'dsar.created');
