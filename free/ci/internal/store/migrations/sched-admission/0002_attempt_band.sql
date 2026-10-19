-- Expand-only. The priority band an attempt was queued in (interactive, normal or background); NULL on rows written before this column, which the world counts as interactive.
ALTER TABLE attempt ADD COLUMN band TEXT;
