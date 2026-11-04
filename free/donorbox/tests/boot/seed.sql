-- Upgrade-proof seed (D-0832): one row per public table the basis Go DDL made, and one per np_donorbox copy the
-- basis migrations made (listed in extra-tables), so the proof shows the converted boot keeps both.
INSERT INTO public.np_donorbox_campaigns (id) VALUES (8320);
INSERT INTO public.np_donorbox_donors (id) VALUES (8320);
INSERT INTO public.np_donorbox_donations (id) VALUES (8320);
INSERT INTO public.np_donorbox_plans (id) VALUES (8320);
INSERT INTO public.np_donorbox_events (id) VALUES (8320);
INSERT INTO public.np_donorbox_tickets (id) VALUES (8320);
INSERT INTO public.np_donorbox_webhook_events (id, event_type) VALUES ('d0832-evt-1', 'donation.created');
INSERT INTO np_donorbox.np_donorbox_campaigns (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_donors (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_donations (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_plans (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_events (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_tickets (id) VALUES (8320);
INSERT INTO np_donorbox.np_donorbox_webhook_events (id, event_type) VALUES ('d0832-evt-1', 'donation.created');
