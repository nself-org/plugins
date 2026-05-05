-- nself-ollama: model registry (B38)
-- Tracks which Ollama models have been pulled and their pull status.

CREATE TABLE IF NOT EXISTS np_ollama_model_registry (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_name  TEXT NOT NULL UNIQUE,     -- e.g. gemma-3-4b, llama3.2:3b
    pulled_at   TIMESTAMPTZ,
    size_bytes  BIGINT,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    status      TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'pulling', 'ready', 'error')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_ollama_model_status
    ON np_ollama_model_registry(status);

-- Seed the default model as 'pending' (pulled on first start by the post-install hook).
INSERT INTO np_ollama_model_registry (model_name, is_default, status)
VALUES ('gemma-3-4b', true, 'pending')
ON CONFLICT (model_name) DO NOTHING;

COMMENT ON TABLE np_ollama_model_registry IS
    'Ollama model registry. The post-install hook auto-pulls the default model '
    '(gemma-3-4b) on first start. Additional models can be pulled via '
    '`nself ollama models pull <name>`.';
