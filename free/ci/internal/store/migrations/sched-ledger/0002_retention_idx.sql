-- Expand-only. The retention delete of each insert (D-1281) seeks these keys instead of scanning the table.
CREATE INDEX ledger_window_day ON ledger(window_day);
CREATE INDEX resource_ledger_window_day ON resource_ledger(window_day);
CREATE INDEX reservation_window ON reservation(window_key);
CREATE INDEX ledger_usage_period ON ledger_usage(period);
