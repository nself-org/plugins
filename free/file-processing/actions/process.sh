#!/bin/bash
# =============================================================================
# File Processing - Process File
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# Parse arguments
if [[ $# -lt 2 ]]; then
    plugin_error "Usage: nself plugin file-processing process <file-id> <file-path>"
    exit 1
fi

FILE_ID="$1"
FILE_PATH="$2"

cd "${PLUGIN_DIR}/ts"

# Check if built
if [[ ! -d "dist" ]]; then
    plugin_error "Plugin not built. Run: nself plugin file-processing init"
    exit 1
fi

# Load environment
if [[ -f ".env" ]]; then
    set -a
    source .env
    set +a
fi

plugin_info "Processing file: ${FILE_PATH}"
plugin_info "File ID: ${FILE_ID}"

# Create job via API
PORT="${PORT:-3104}"
FILENAME=$(basename "${FILE_PATH}")

# Get file size and MIME type
if [[ -f "${FILE_PATH}" ]]; then
    FILE_SIZE=$(stat -f%z "${FILE_PATH}" 2>/dev/null || stat -c%s "${FILE_PATH}" 2>/dev/null || echo "0")
    MIME_TYPE=$(file --mime-type -b "${FILE_PATH}" 2>/dev/null || echo "application/octet-stream")
else
    plugin_warn "File not found locally, using defaults"
    FILE_SIZE=0
    MIME_TYPE="application/octet-stream"
fi

# Create job
RESPONSE=$(curl -s -X POST "http://localhost:${PORT}/api/jobs" \
    -H "Content-Type: application/json" \
    -d "{
        \"fileId\": \"${FILE_ID}\",
        \"filePath\": \"${FILE_PATH}\",
        \"fileName\": \"${FILENAME}\",
        \"fileSize\": ${FILE_SIZE},
        \"mimeType\": \"${MIME_TYPE}\",
        \"operations\": [\"thumbnail\", \"optimize\", \"metadata\"]
    }" 2>&1)

if [[ $? -eq 0 ]]; then
    JOB_ID=$(echo "${RESPONSE}" | grep -o '"jobId":"[^"]*"' | cut -d'"' -f4)
    if [[ -n "${JOB_ID}" ]]; then
        plugin_success "Job created: ${JOB_ID}"
        plugin_info "Check status: curl http://localhost:${PORT}/api/jobs/${JOB_ID}"
    else
        plugin_error "Failed to create job"
        echo "${RESPONSE}"
        exit 1
    fi
else
    plugin_error "Failed to connect to server (is it running?)"
    plugin_info "Start server: nself plugin file-processing server"
    exit 1
fi
