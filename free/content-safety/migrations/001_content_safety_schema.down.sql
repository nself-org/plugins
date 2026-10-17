-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
DROP TABLE IF EXISTS public.np_cs_abuse_scores;
DROP TABLE IF EXISTS public.np_cs_lockdowns;
DROP TABLE IF EXISTS public.np_cs_raid_events;
DROP TABLE IF EXISTS public.np_cs_rate_limit_violations;
DROP TABLE IF EXISTS public.np_cs_spam_configs;
DROP TABLE IF EXISTS public.np_cs_spam_rules;
DROP TABLE IF EXISTS public.np_cs_evidence_exports;
DROP TABLE IF EXISTS public.np_cs_legal_holds;
DROP TABLE IF EXISTS public.np_cs_evidence;
