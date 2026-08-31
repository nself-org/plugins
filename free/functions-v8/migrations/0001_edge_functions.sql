-- B25: Edge Functions V8 Runtime — initial schema
-- np_ prefix enforced per nSelf plugin conventions

CREATE TABLE IF NOT EXISTS np_edge_functions (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name              text UNIQUE NOT NULL,
  description       text,
  status            text NOT NULL DEFAULT 'active',  -- active | paused | error
  invoke_count      bigint NOT NULL DEFAULT 0,
  error_count       bigint NOT NULL DEFAULT 0,
  source_account_id text NOT NULL DEFAULT 'primary',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS np_edge_function_versions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  function_id uuid NOT NULL REFERENCES np_edge_functions(id) ON DELETE CASCADE,
  version     int NOT NULL,
  source_code text NOT NULL,
  is_active   bool NOT NULL DEFAULT false,
  deployed_at timestamptz NOT NULL DEFAULT now(),
  deployed_by text,
  UNIQUE (function_id, version)
);

CREATE TABLE IF NOT EXISTS np_edge_function_env (
  function_id uuid NOT NULL REFERENCES np_edge_functions(id) ON DELETE CASCADE,
  key         text NOT NULL,
  value       text NOT NULL,
  PRIMARY KEY (function_id, key)
);

CREATE TABLE IF NOT EXISTS np_function_logs (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  function_id uuid REFERENCES np_edge_functions(id) ON DELETE CASCADE,
  level       text NOT NULL DEFAULT 'info',
  message     text NOT NULL,
  metadata    jsonb,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS np_function_logs_fn_created
  ON np_function_logs (function_id, created_at DESC);

-- Ensure only one active version per function
CREATE UNIQUE INDEX IF NOT EXISTS np_edge_function_versions_active_unique
  ON np_edge_function_versions (function_id)
  WHERE is_active = true;
