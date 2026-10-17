-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback for 001_init.sql
-- Drops tables in reverse dependency order.

DROP TABLE IF EXISTS public.meetings_waitlist;
DROP TABLE IF EXISTS public.meetings_reminders;
DROP TABLE IF EXISTS public.meetings_templates;
DROP TABLE IF EXISTS public.meetings_external_calendars;
DROP TABLE IF EXISTS public.meetings_calendar_shares;
DROP TABLE IF EXISTS public.meetings_attendees;
DROP TABLE IF EXISTS public.meetings_events;
DROP TABLE IF EXISTS public.meetings_rooms;
DROP TABLE IF EXISTS public.meetings_calendars;
