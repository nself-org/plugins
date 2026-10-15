-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
DROP TABLE IF EXISTS np_sms_opt_outs;
DROP TABLE IF EXISTS np_sms_messages;
