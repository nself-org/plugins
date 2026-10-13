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
