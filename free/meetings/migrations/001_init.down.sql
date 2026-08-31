-- Rollback for 001_init.sql
-- Drops tables in reverse dependency order.

DROP TABLE IF EXISTS meetings_waitlist;
DROP TABLE IF EXISTS meetings_reminders;
DROP TABLE IF EXISTS meetings_templates;
DROP TABLE IF EXISTS meetings_external_calendars;
DROP TABLE IF EXISTS meetings_calendar_shares;
DROP TABLE IF EXISTS meetings_attendees;
DROP TABLE IF EXISTS meetings_events;
DROP TABLE IF EXISTS meetings_rooms;
DROP TABLE IF EXISTS meetings_calendars;
