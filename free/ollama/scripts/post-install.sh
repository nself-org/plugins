#!/bin/sh
# nself-ollama: post-install hook (B38)
# Runs after `nself plugin install ollama`.
# Pulls the default model on first start when NSELF_OLLAMA_AUTO_PULL=true.
# Bash 3.2 compatible: no echo -e, no ${var,,}, no declare -A.

set -e

OLLAMA_HOST="${NSELF_OLLAMA_HOST:-http://ollama:11434}"
MODEL="${NSELF_OLLAMA_DEFAULT_MODEL:-gemma-3-4b}"
AUTO_PULL="${NSELF_OLLAMA_AUTO_PULL:-true}"

if [ "$AUTO_PULL" != "true" ]; then
    printf '[ollama] auto-pull disabled (NSELF_OLLAMA_AUTO_PULL=false)\n'
    exit 0
fi

printf '[ollama] waiting for Ollama service to be ready...\n'
RETRIES=12
i=0
while [ "$i" -lt "$RETRIES" ]; do
    if curl -sf "${OLLAMA_HOST}/api/version" > /dev/null 2>&1; then
        break
    fi
    i=$((i + 1))
    printf '[ollama] not ready yet, retry %d/%d\n' "$i" "$RETRIES"
    sleep 5
done

if ! curl -sf "${OLLAMA_HOST}/api/version" > /dev/null 2>&1; then
    printf '[ollama] ERROR: service did not become ready after %d retries\n' "$RETRIES"
    exit 1
fi

# Check if model already pulled
STATUS=$(curl -sf "${OLLAMA_HOST}/api/tags" 2>/dev/null || printf '{}')
if printf '%s' "$STATUS" | grep -q "\"${MODEL}\""; then
    printf '[ollama] model %s already present\n' "$MODEL"
    exit 0
fi

printf '[ollama] pulling model %s (this may take several minutes)...\n' "$MODEL"
curl -sf -X POST "${OLLAMA_HOST}/api/pull" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${MODEL}\"}" | while IFS= read -r line; do
    printf '[ollama] %s\n' "$line"
done

printf '[ollama] model %s ready\n' "$MODEL"
