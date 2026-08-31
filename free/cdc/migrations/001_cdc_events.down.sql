-- 001_cdc_events.down.sql
-- Reverse of 001_cdc_events.sql
-- Drops CDC infrastructure: event buffer table, publication, and replication slot.

-- 1. Drop the event buffer table (indexes dropped automatically with the table).
DROP TABLE IF EXISTS np_cdc_events;

-- 2. Drop the publication.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'nself_pub') THEN
        DROP PUBLICATION nself_pub;
    END IF;
END $$;

-- 3. Drop the logical replication slot.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'nself_cdc') THEN
        PERFORM pg_drop_replication_slot('nself_cdc');
    END IF;
END $$;
