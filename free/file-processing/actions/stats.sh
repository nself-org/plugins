#!/bin/bash
# =============================================================================
# File Processing - Statistics
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

plugin_info "File Processing Statistics"
echo ""

# Overall stats
plugin_info "Job Status:"
plugin_db_exec "
SELECT
    status,
    COUNT(*) AS count,
    ROUND(AVG(duration_ms)) AS avg_duration_ms
FROM file_processing_jobs
GROUP BY status
ORDER BY
    CASE status
        WHEN 'processing' THEN 1
        WHEN 'pending' THEN 2
        WHEN 'completed' THEN 3
        WHEN 'failed' THEN 4
        WHEN 'cancelled' THEN 5
    END;
" | column -t

echo ""

# Thumbnail stats
plugin_info "Thumbnail Generation:"
plugin_db_exec "
SELECT
    width || 'x' || height AS size,
    format,
    COUNT(*) AS count,
    ROUND(AVG(generation_time_ms)) AS avg_time_ms,
    pg_size_pretty(SUM(size_bytes)::BIGINT) AS total_size
FROM file_thumbnails
GROUP BY width, height, format
ORDER BY width;
" | column -t

echo ""

# Scan stats (if enabled)
SCAN_COUNT=$(plugin_db_exec "SELECT COUNT(*) FROM file_scans;" | tail -n 1 | tr -d ' ')
if [[ "${SCAN_COUNT}" -gt 0 ]]; then
    plugin_info "Virus Scans:"
    plugin_db_exec "
    SELECT
        scan_status AS status,
        COUNT(*) AS count,
        ROUND(AVG(scan_duration_ms)) AS avg_duration_ms
    FROM file_scans
    GROUP BY scan_status
    ORDER BY count DESC;
    " | column -t
    echo ""
fi

# Recent failures
FAILURE_COUNT=$(plugin_db_exec "SELECT COUNT(*) FROM file_processing_failures;" | tail -n 1 | tr -d ' ')
if [[ "${FAILURE_COUNT}" -gt 0 ]]; then
    plugin_warn "Recent Failures: ${FAILURE_COUNT}"
    plugin_db_exec "
    SELECT
        file_name,
        status,
        attempts,
        error_message,
        last_error_at
    FROM file_processing_failures
    LIMIT 10;
    " | column -t
    echo ""
fi

plugin_success "Statistics complete"
