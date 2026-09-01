#!/bin/bash
# =============================================================================
# File Processing - Initialize
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

plugin_info "Initializing File Processing plugin..."

# Check dependencies
plugin_info "Checking dependencies..."

if ! command -v node >/dev/null 2>&1; then
    plugin_error "Node.js is required but not installed"
    exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
    plugin_error "npm is required but not installed"
    exit 1
fi

# Install and build TypeScript
cd "${PLUGIN_DIR}/ts"

if [[ ! -d "node_modules" ]]; then
    plugin_info "Installing dependencies..."
    npm install
fi

if [[ ! -d "dist" ]]; then
    plugin_info "Building TypeScript..."
    npm run build
fi

# Check optional dependencies
if command -v ffmpeg >/dev/null 2>&1; then
    plugin_success "✓ ffmpeg found (video thumbnails enabled)"
else
    plugin_warn "⚠ ffmpeg not found (video thumbnails disabled)"
fi

if command -v clamdscan >/dev/null 2>&1 || command -v clamd >/dev/null 2>&1; then
    plugin_success "✓ ClamAV found (virus scanning available)"
else
    plugin_warn "⚠ ClamAV not found (virus scanning disabled)"
fi

plugin_success "File Processing plugin initialized!"
plugin_info "Start server: nself plugin file-processing server"
plugin_info "Start worker: nself plugin file-processing worker"
