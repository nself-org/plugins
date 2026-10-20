-- Expand-only (D-1272). The identity a pipeline was submitted under, read by the world builder to count
-- active attempts per user and per team. Empty means unknown: the builder then keeps the global upper bound.
ALTER TABLE pipeline ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline ADD COLUMN team_id TEXT NOT NULL DEFAULT '';
