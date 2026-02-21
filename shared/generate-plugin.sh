#!/usr/bin/env bash
# =============================================================================
# nself Plugin Template Generator
# Generates a complete plugin scaffold under free/<plugin-name>/
#
# Usage:
#   ./generate-plugin.sh <plugin-name> <category> [--description "..."] [--author "..."]
#
# Example:
#   ./generate-plugin.sh my-service integrations --description "My service integration" --author "nself"
#
# Bash 3.2 compatible (macOS default shell):
#   - No echo -e  (use printf)
#   - No ${var,,} or ${var^^}  (use tr)
#   - No {1..N} in for loops  (use seq)
#   - No declare -A associative arrays
# =============================================================================

set -e

# =============================================================================
# Color Codes (printf-safe)
# =============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# =============================================================================
# Logging Helpers
# =============================================================================

log_info() {
    printf "${BLUE}  →${RESET}  %s\n" "$*"
}

log_success() {
    printf "${GREEN}  ✓${RESET}  %s\n" "$*"
}

log_warn() {
    printf "${YELLOW}  ⚠${RESET}  %s\n" "$*"
}

log_error() {
    printf "${RED}  ✗${RESET}  %s\n" "$*" >&2
}

log_header() {
    printf "\n${BOLD}${CYAN}%s${RESET}\n" "$*"
    printf "${CYAN}%s${RESET}\n" "$(printf '=%.0s' $(seq 1 ${#1}))"
}

log_step() {
    printf "${BOLD}  [%s]${RESET} %s\n" "$1" "$2"
}

# =============================================================================
# Utility Functions — Bash 3.2 Compatible
# =============================================================================

# Convert kebab-case to PascalCase: "my-plugin" → "MyPlugin"
to_pascal_case() {
    local input="$1"
    local result=""
    local word=""

    # Replace hyphens with spaces, then capitalize each word
    local spaced
    spaced=$(printf '%s' "$input" | tr '-' ' ')

    for word in $spaced; do
        # Capitalize first letter using tr (Bash 3.2 compatible)
        local first_char
        first_char=$(printf '%s' "$word" | cut -c1 | tr '[:lower:]' '[:upper:]')
        local rest
        rest=$(printf '%s' "$word" | cut -c2-)
        result="${result}${first_char}${rest}"
    done

    printf '%s' "$result"
}

# Convert kebab-case to SCREAMING_SNAKE_CASE: "my-plugin" → "MY_PLUGIN"
to_upper_snake() {
    printf '%s' "$1" | tr '-' '_' | tr '[:lower:]' '[:upper:]'
}

# Convert kebab-case to lowercase with underscores: "my-plugin" → "my_plugin"
to_snake_case() {
    printf '%s' "$1" | tr '-' '_' | tr '[:upper:]' '[:lower:]'
}

# Strip hyphens entirely: "my-plugin" → "myplugin"
strip_hyphens() {
    printf '%s' "$1" | tr -d '-'
}

# =============================================================================
# Validation
# =============================================================================

VALID_CATEGORIES="authentication automation commerce communication content data development infrastructure integrations media streaming sports compliance"

validate_plugin_name() {
    local name="$1"

    # Must be lowercase letters, numbers, and hyphens only
    case "$name" in
        *[A-Z]*)
            log_error "Plugin name must be lowercase. Got: $name"
            return 1
            ;;
        *_*)
            log_error "Plugin name must use hyphens, not underscores. Got: $name"
            return 1
            ;;
        *[^a-z0-9-]*)
            log_error "Plugin name may only contain lowercase letters, digits, and hyphens. Got: $name"
            return 1
            ;;
        -* | *-)
            log_error "Plugin name cannot start or end with a hyphen. Got: $name"
            return 1
            ;;
    esac

    if [ ${#name} -lt 2 ]; then
        log_error "Plugin name must be at least 2 characters long."
        return 1
    fi

    if [ ${#name} -gt 64 ]; then
        log_error "Plugin name must be 64 characters or fewer."
        return 1
    fi

    return 0
}

validate_category() {
    local category="$1"
    local cat

    for cat in $VALID_CATEGORIES; do
        if [ "$cat" = "$category" ]; then
            return 0
        fi
    done

    log_error "Invalid category: '$category'"
    log_error "Must be one of: $VALID_CATEGORIES"
    return 1
}

# =============================================================================
# Argument Parsing
# =============================================================================

usage() {
    printf "\n${BOLD}Usage:${RESET}\n"
    printf "  %s <plugin-name> <category> [options]\n\n" "$(basename "$0")"
    printf "${BOLD}Arguments:${RESET}\n"
    printf "  plugin-name   Lowercase, hyphens only (e.g. my-service)\n"
    printf "  category      One of: %s\n\n" "$VALID_CATEGORIES"
    printf "${BOLD}Options:${RESET}\n"
    printf "  --description \"...\"   Plugin description (default: <plugin-name> integration)\n"
    printf "  --author \"...\"        Plugin author (default: nself)\n"
    printf "  --help                Show this help\n\n"
    printf "${BOLD}Example:${RESET}\n"
    printf "  %s my-service integrations --description \"My Service integration for nself\" --author \"Your Name\"\n\n" "$(basename "$0")"
}

PLUGIN_NAME=""
CATEGORY=""
DESCRIPTION=""
AUTHOR="nself"

parse_args() {
    if [ $# -lt 2 ]; then
        log_error "Missing required arguments."
        usage
        exit 1
    fi

    PLUGIN_NAME="$1"
    CATEGORY="$2"
    shift 2

    while [ $# -gt 0 ]; do
        case "$1" in
            --description)
                if [ -z "${2:-}" ]; then
                    log_error "--description requires a value"
                    exit 1
                fi
                DESCRIPTION="$2"
                shift 2
                ;;
            --author)
                if [ -z "${2:-}" ]; then
                    log_error "--author requires a value"
                    exit 1
                fi
                AUTHOR="$2"
                shift 2
                ;;
            --help | -h)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown argument: $1"
                usage
                exit 1
                ;;
        esac
    done

    # Set default description if not provided
    if [ -z "$DESCRIPTION" ]; then
        DESCRIPTION="${PLUGIN_NAME} integration for nself"
    fi
}

# =============================================================================
# Derived Name Variants (computed after arg parsing)
# =============================================================================

compute_names() {
    PASCAL_NAME=$(to_pascal_case "$PLUGIN_NAME")         # MyPlugin
    SNAKE_NAME=$(to_snake_case "$PLUGIN_NAME")            # my_plugin
    UPPER_NAME=$(to_upper_snake "$PLUGIN_NAME")           # MY_PLUGIN
    TABLE_PREFIX="np_${SNAKE_NAME}_"                      # np_my_plugin_
    # Strip hyphens for internal short abbreviation used in log namespaces
    SHORT_NAME=$(strip_hyphens "$PLUGIN_NAME")            # myplugin
}

# =============================================================================
# Target Directory
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGINS_ROOT="$(dirname "$SCRIPT_DIR")"   # nself-plugins root (shared/../)
FREE_DIR="${PLUGINS_ROOT}/free"
TARGET_DIR=""

setup_target_dir() {
    TARGET_DIR="${FREE_DIR}/${PLUGIN_NAME}"

    if [ -d "$TARGET_DIR" ]; then
        log_error "Plugin directory already exists: $TARGET_DIR"
        log_error "Delete it first or choose a different plugin name."
        exit 1
    fi

    if [ ! -d "$FREE_DIR" ]; then
        log_error "Expected free plugins directory not found: $FREE_DIR"
        log_error "Run this script from inside the nself-plugins/shared/ directory."
        exit 1
    fi
}

# =============================================================================
# File Generation Helpers
# =============================================================================

make_dir() {
    mkdir -p "$1"
    log_success "Created directory: ${1#"$PLUGINS_ROOT/"}"
}

write_file() {
    local path="$1"
    # Content is passed via heredoc from caller via stdin
    cat > "$path"
    log_success "Created: ${path#"$PLUGINS_ROOT/"}"
}

# =============================================================================
# Template: README.md
# =============================================================================

generate_readme() {
    write_file "${TARGET_DIR}/README.md" <<README
# ${PASCAL_NAME} Plugin for nself

${DESCRIPTION}

## Features

- **Full Data Sync** — Sync ${PLUGIN_NAME} data to PostgreSQL
- **Real-time Webhooks** — Handle incoming ${PLUGIN_NAME} events
- **REST API** — Query synced data via HTTP endpoints
- **CLI Tools** — Command-line interface for management

## Installation

\`\`\`bash
nself plugin install ${PLUGIN_NAME}
\`\`\`

## Configuration

Set the following environment variables before installing:

\`\`\`bash
# Required
${UPPER_NAME}_API_KEY=your_api_key_here
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional
${UPPER_NAME}_WEBHOOK_SECRET=your_webhook_secret
${UPPER_NAME}_PLUGIN_PORT=3000
\`\`\`

## Usage

\`\`\`bash
# Sync data
nself plugin ${PLUGIN_NAME} sync

# Start webhook server
nself plugin ${PLUGIN_NAME} server

# Check status
nself plugin ${PLUGIN_NAME} status
\`\`\`

## Tables

All data is stored with the \`${TABLE_PREFIX}\` prefix:

| Table | Description |
|-------|-------------|
| \`${TABLE_PREFIX}records\` | Main ${PLUGIN_NAME} records |
| \`${TABLE_PREFIX}webhook_events\` | Incoming webhook events |

## Webhooks

Configure your ${PASCAL_NAME} webhook to point to:

\`\`\`
https://your-domain.com/webhooks/${PLUGIN_NAME}
\`\`\`

## Development

\`\`\`bash
# Install dependencies
cd ts && pnpm install

# Build TypeScript
pnpm run build

# Development mode (watch)
pnpm run dev

# Type check
pnpm run typecheck
\`\`\`

## License

MIT
README
}

# =============================================================================
# Template: plugin.json
# =============================================================================

generate_plugin_json() {
    write_file "${TARGET_DIR}/plugin.json" <<PLUGINJSON
{
  "name": "${PLUGIN_NAME}",
  "version": "1.0.0",
  "description": "${DESCRIPTION}",
  "author": "${AUTHOR}",
  "license": "MIT",
  "homepage": "https://github.com/nself-org/plugins/tree/main/free/${PLUGIN_NAME}",
  "repository": "https://github.com/nself-org/plugins",
  "minNselfVersion": "0.4.8",
  "category": "${CATEGORY}",
  "tags": [
    "${PLUGIN_NAME}"
  ],
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  },
  "tables": [
    "${TABLE_PREFIX}records",
    "${TABLE_PREFIX}webhook_events"
  ],
  "webhooks": {
    "created": "Resource created",
    "updated": "Resource updated",
    "deleted": "Resource deleted"
  },
  "actions": {
    "sync": "Sync data from ${PASCAL_NAME}",
    "status": "Show sync status",
    "webhook": "Manage webhook events"
  },
  "envVars": {
    "required": [
      "DATABASE_URL",
      "${UPPER_NAME}_API_KEY"
    ],
    "optional": [
      "${UPPER_NAME}_WEBHOOK_SECRET",
      "${UPPER_NAME}_PLUGIN_PORT"
    ]
  },
  "config": {
    "webhookPath": "/webhooks/${PLUGIN_NAME}",
    "syncInterval": 3600
  },
  "port": 3000
}
PLUGINJSON
}

# =============================================================================
# Template: install.sh
# =============================================================================

generate_install_sh() {
    write_file "${TARGET_DIR}/install.sh" <<INSTALLSH
#!/usr/bin/env bash
# ${PASCAL_NAME} Plugin - Installation Script

set -e

PLUGIN_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="${PLUGIN_NAME}"

# Source shared utilities
source "\${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Pre-flight Checks
# ============================================================================

preflight_checks() {
    plugin_log "info" "Running pre-flight checks..."

    if [[ -z "\${${UPPER_NAME}_API_KEY:-}" ]]; then
        plugin_log "error" "${UPPER_NAME}_API_KEY is required"
        plugin_log "info" "Set ${UPPER_NAME}_API_KEY in your environment before installing"
        return 1
    fi

    plugin_log "success" "Pre-flight checks passed"
    return 0
}

# ============================================================================
# Database Setup
# ============================================================================

setup_database() {
    plugin_log "info" "Setting up database schema..."

    if [[ -f "\${PLUGIN_DIR}/schema/${PLUGIN_NAME}.sql" ]]; then
        plugin_log "info" "Applying database schema..."
        plugin_db_exec_file "\${PLUGIN_DIR}/schema/${PLUGIN_NAME}.sql"
        plugin_log "success" "Database schema applied"
    else
        plugin_log "error" "Schema file not found: \${PLUGIN_DIR}/schema/${PLUGIN_NAME}.sql"
        return 1
    fi

    return 0
}

# ============================================================================
# Webhook Setup
# ============================================================================

setup_webhooks() {
    plugin_log "info" "Configuring webhook handlers..."

    local webhook_path="\${${UPPER_NAME}_WEBHOOK_PATH:-/webhooks/${PLUGIN_NAME}}"
    plugin_log "info" "Webhook endpoint: \$webhook_path"

    if [[ -n "\${${UPPER_NAME}_WEBHOOK_SECRET:-}" ]]; then
        plugin_log "success" "Webhook signature verification enabled"
    else
        plugin_log "warning" "${UPPER_NAME}_WEBHOOK_SECRET not set — webhook signatures won't be verified"
        plugin_log "info" "Set ${UPPER_NAME}_WEBHOOK_SECRET for production use"
    fi

    local webhook_runtime_dir="\${HOME}/.nself/plugins/\${PLUGIN_NAME}/webhooks"
    mkdir -p "\$webhook_runtime_dir"

    if [[ -d "\${PLUGIN_DIR}/webhooks" ]]; then
        cp -r "\${PLUGIN_DIR}/webhooks/"* "\$webhook_runtime_dir/" 2>/dev/null || true
        chmod +x "\$webhook_runtime_dir"/*.sh 2>/dev/null || true
    fi

    return 0
}

# ============================================================================
# Main Installation
# ============================================================================

main() {
    plugin_log "info" "Installing ${PASCAL_NAME} plugin..."

    if ! preflight_checks; then
        plugin_log "error" "Pre-flight checks failed"
        return 1
    fi

    if ! setup_database; then
        plugin_log "error" "Database setup failed"
        return 1
    fi

    if ! setup_webhooks; then
        plugin_log "error" "Webhook setup failed"
        return 1
    fi

    plugin_mark_installed "\$PLUGIN_NAME"

    plugin_log "success" "${PASCAL_NAME} plugin installed successfully"

    printf "\n"
    printf "Usage:\n"
    printf "  nself plugin ${PLUGIN_NAME} sync         - Sync data\n"
    printf "  nself plugin ${PLUGIN_NAME} status       - Show status\n"
    printf "  nself plugin ${PLUGIN_NAME} webhook      - Manage webhooks\n"
    printf "\n"

    return 0
}

if [[ "\${BASH_SOURCE[0]}" == "\${0}" ]]; then
    main "\$@"
fi
INSTALLSH
    chmod +x "${TARGET_DIR}/install.sh"
}

# =============================================================================
# Template: uninstall.sh
# =============================================================================

generate_uninstall_sh() {
    write_file "${TARGET_DIR}/uninstall.sh" <<UNINSTALLSH
#!/usr/bin/env bash
# ${PASCAL_NAME} Plugin - Uninstall Script

set -e

PLUGIN_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="${PLUGIN_NAME}"

# Source shared utilities
source "\${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Uninstallation
# ============================================================================

remove_database() {
    local keep_data="\${1:-false}"

    if [[ "\$keep_data" == "true" ]]; then
        plugin_log "info" "Keeping database tables (--keep-data specified)"
        return 0
    fi

    plugin_log "warning" "Removing ${PASCAL_NAME} database tables..."

    plugin_db_query "DROP TABLE IF EXISTS ${TABLE_PREFIX}webhook_events CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS ${TABLE_PREFIX}records CASCADE" 2>/dev/null || true

    plugin_log "success" "Database tables removed"
    return 0
}

remove_webhooks() {
    plugin_log "info" "Removing webhook handlers..."

    local webhook_runtime_dir="\${HOME}/.nself/plugins/\${PLUGIN_NAME}/webhooks"

    if [[ -d "\$webhook_runtime_dir" ]]; then
        rm -rf "\$webhook_runtime_dir"
        plugin_log "success" "Webhook handlers removed"
    fi

    return 0
}

remove_cache() {
    plugin_log "info" "Clearing plugin cache..."

    local cache_dir="\${HOME}/.nself/cache/plugins/\${PLUGIN_NAME}"

    if [[ -d "\$cache_dir" ]]; then
        rm -rf "\$cache_dir"
        plugin_log "success" "Cache cleared"
    fi

    return 0
}

main() {
    local keep_data=false

    while [[ \$# -gt 0 ]]; do
        case "\$1" in
            --keep-data)
                keep_data=true
                shift
                ;;
            *)
                shift
                ;;
        esac
    done

    plugin_log "info" "Uninstalling ${PASCAL_NAME} plugin..."

    remove_database "\$keep_data"
    remove_webhooks
    remove_cache

    plugin_mark_uninstalled "\$PLUGIN_NAME"

    plugin_log "success" "${PASCAL_NAME} plugin uninstalled"

    if [[ "\$keep_data" == "true" ]]; then
        printf "\n"
        printf "Note: Database tables were preserved. To remove them later:\n"
        printf "  nself db query \"DROP TABLE IF EXISTS ${TABLE_PREFIX}* CASCADE\"\n"
    fi

    return 0
}

if [[ "\${BASH_SOURCE[0]}" == "\${0}" ]]; then
    main "\$@"
fi
UNINSTALLSH
    chmod +x "${TARGET_DIR}/uninstall.sh"
}

# =============================================================================
# Template: schema/<plugin-name>.sql
# =============================================================================

generate_schema_sql() {
    write_file "${TARGET_DIR}/schema/${PLUGIN_NAME}.sql" <<SCHEMASQL
-- =============================================================================
-- ${PASCAL_NAME} Plugin Schema
-- Table prefix: ${TABLE_PREFIX}
-- All tables include source_account_id for multi-app isolation
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Main Records Table
-- =============================================================================

CREATE TABLE IF NOT EXISTS ${TABLE_PREFIX}records (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id   VARCHAR(255) NOT NULL DEFAULT 'primary',
    external_id         VARCHAR(255),                           -- ID from external service
    name                VARCHAR(255),
    description         TEXT,
    status              VARCHAR(50),
    data                JSONB DEFAULT '{}',                     -- Raw API data
    created_at          TIMESTAMP WITH TIME ZONE,
    updated_at          TIMESTAMP WITH TIME ZONE,
    synced_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_records_ext_id
    ON ${TABLE_PREFIX}records (source_account_id, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_records_account
    ON ${TABLE_PREFIX}records (source_account_id);

CREATE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_records_updated
    ON ${TABLE_PREFIX}records (updated_at DESC);

-- =============================================================================
-- Webhook Events Table
-- =============================================================================

CREATE TABLE IF NOT EXISTS ${TABLE_PREFIX}webhook_events (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id   VARCHAR(255) NOT NULL DEFAULT 'primary',
    event_type          VARCHAR(100) NOT NULL,
    action              VARCHAR(100),
    external_id         VARCHAR(255),                           -- Resource ID from payload
    data                JSONB NOT NULL DEFAULT '{}',            -- Full webhook payload
    processed           BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at        TIMESTAMP WITH TIME ZONE,
    error               TEXT,
    received_at         TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_wh_events_account
    ON ${TABLE_PREFIX}webhook_events (source_account_id);

CREATE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_wh_events_type
    ON ${TABLE_PREFIX}webhook_events (event_type);

CREATE INDEX IF NOT EXISTS idx_${SNAKE_NAME}_wh_events_processed
    ON ${TABLE_PREFIX}webhook_events (processed, received_at DESC)
    WHERE processed = FALSE;

-- =============================================================================
-- Helpful Views
-- =============================================================================

CREATE OR REPLACE VIEW ${SNAKE_NAME}_recent_events AS
SELECT
    id,
    source_account_id,
    event_type,
    action,
    external_id,
    processed,
    received_at,
    processed_at
FROM ${TABLE_PREFIX}webhook_events
ORDER BY received_at DESC;

CREATE OR REPLACE VIEW ${SNAKE_NAME}_unprocessed_events AS
SELECT *
FROM ${TABLE_PREFIX}webhook_events
WHERE processed = FALSE
ORDER BY received_at ASC;
SCHEMASQL
}

# =============================================================================
# Template: templates/env.template
# =============================================================================

generate_env_template() {
    write_file "${TARGET_DIR}/templates/env.template" <<ENVTEMPLATE
# =============================================================================
# ${PASCAL_NAME} Plugin — Environment Variables
# Copy relevant variables to your .env file
# =============================================================================

# ---------------------------------------------------------------------------
# Required
# ---------------------------------------------------------------------------

# ${PASCAL_NAME} API credentials
${UPPER_NAME}_API_KEY=your_api_key_here

# Database (usually inherited from nself)
DATABASE_URL=postgresql://user:password@localhost:5432/nself

# ---------------------------------------------------------------------------
# Optional
# ---------------------------------------------------------------------------

# Webhook signature verification secret (strongly recommended for production)
${UPPER_NAME}_WEBHOOK_SECRET=

# Server configuration
${UPPER_NAME}_PLUGIN_PORT=3000
${UPPER_NAME}_PLUGIN_HOST=0.0.0.0

# Database connection (if not using DATABASE_URL)
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=postgres
POSTGRES_PASSWORD=
POSTGRES_SSL=false

# Sync configuration
${UPPER_NAME}_SYNC_INTERVAL=3600

# Multi-app support: comma-separated key:label pairs
# ${UPPER_NAME}_API_KEYS=key1:account1,key2:account2
# ${UPPER_NAME}_ACCOUNT_LABELS=primary,secondary

# Logging
LOG_LEVEL=info
ENVTEMPLATE
}

# =============================================================================
# Template: ts/package.json
# =============================================================================

generate_ts_package_json() {
    write_file "${TARGET_DIR}/ts/package.json" <<PKGJSON
{
  "name": "@nself/plugin-${PLUGIN_NAME}",
  "version": "1.0.0",
  "description": "${DESCRIPTION}",
  "type": "module",
  "main": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "bin": {
    "nself-${PLUGIN_NAME}": "./dist/cli.js"
  },
  "scripts": {
    "build": "tsc",
    "watch": "tsc --watch",
    "start": "node dist/server.js",
    "dev": "tsx watch src/server.ts",
    "sync": "node dist/cli.js sync",
    "clean": "rm -rf dist",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "@nself/plugin-utils": "file:../../../shared",
    "fastify": "^4.24.0",
    "@fastify/cors": "^8.4.0",
    "dotenv": "^16.3.1",
    "commander": "^11.1.0",
    "pg": "^8.11.3"
  },
  "devDependencies": {
    "@types/node": "^20.10.0",
    "typescript": "^5.3.0",
    "tsx": "^4.6.0"
  },
  "engines": {
    "node": ">=18.0.0"
  },
  "license": "MIT"
}
PKGJSON
}

# =============================================================================
# Template: ts/tsconfig.json
# =============================================================================

generate_ts_tsconfig() {
    write_file "${TARGET_DIR}/ts/tsconfig.json" <<TSCONFIG
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "lib": ["ES2022"],
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,
    "noEmitOnError": true,
    "noImplicitAny": true,
    "noImplicitReturns": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
TSCONFIG
}

# =============================================================================
# Template: ts/src/types.ts
# =============================================================================

generate_ts_types() {
    write_file "${TARGET_DIR}/ts/src/types.ts" <<TYPES
/**
 * ${PASCAL_NAME} Plugin Types
 */

// =============================================================================
// Plugin Configuration
// =============================================================================

export interface ${PASCAL_NAME}PluginConfig {
  apiKey: string;
  webhookSecret?: string;
  port: number;
  host: string;
  syncInterval?: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// API Response Types
// Adapt these to match the actual ${PASCAL_NAME} API responses
// =============================================================================

export interface ${PASCAL_NAME}ApiRecord {
  id: string;
  name?: string;
  description?: string | null;
  status?: string;
  created_at?: string;
  updated_at?: string;
  [key: string]: unknown;
}

export interface ${PASCAL_NAME}WebhookPayload {
  event: string;
  action?: string;
  id?: string;
  data: Record<string, unknown>;
  timestamp?: string;
}

// =============================================================================
// Database Record Types
// source_account_id is REQUIRED on every table for multi-app isolation
// =============================================================================

export interface ${PASCAL_NAME}Record {
  id: string;                       // UUID primary key
  source_account_id: string;        // Multi-app isolation — NEVER omit
  external_id: string | null;       // ID from the external ${PASCAL_NAME} service
  name: string | null;
  description: string | null;
  status: string | null;
  data: Record<string, unknown>;    // Raw API data (JSONB)
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface ${PASCAL_NAME}WebhookEventRecord {
  id: string;                       // UUID primary key
  source_account_id: string;        // Multi-app isolation — NEVER omit
  event_type: string;
  action: string | null;
  external_id: string | null;       // Resource ID from payload
  data: Record<string, unknown>;    // Full webhook payload (JSONB)
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  received_at: Date;
}

// =============================================================================
// Sync Types
// =============================================================================

export interface SyncStats {
  records: number;
  webhookEvents?: number;
  lastSyncedAt?: Date | null;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  limit?: number;
}

export interface SyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
}
TYPES
}

# =============================================================================
# Template: ts/src/config.ts
# =============================================================================

generate_ts_config() {
    write_file "${TARGET_DIR}/ts/src/config.ts" <<CONFIG
/**
 * ${PASCAL_NAME} Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, buildAccountConfigs, type SecurityConfig, type AccountConfig } from '@nself/plugin-utils';

export interface Config {
  // ${PASCAL_NAME} credentials
  apiKey: string;
  webhookSecret: string;

  // Multi-app support
  accounts: AccountConfig[];

  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Sync
  syncInterval: number;
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('${UPPER_NAME}');

  const accounts = buildAccountConfigs(
    process.env.${UPPER_NAME}_API_KEYS,
    process.env.${UPPER_NAME}_ACCOUNT_LABELS,
    process.env.${UPPER_NAME}_API_KEY,
    'primary'
  );

  const config: Config = {
    // ${PASCAL_NAME} credentials
    apiKey: process.env.${UPPER_NAME}_API_KEY ?? '',
    webhookSecret: process.env.${UPPER_NAME}_WEBHOOK_SECRET ?? '',

    // Multi-app support
    accounts,

    // Server
    port: parseInt(process.env.${UPPER_NAME}_PLUGIN_PORT ?? process.env.PORT ?? '3000', 10),
    host: process.env.${UPPER_NAME}_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Sync
    syncInterval: parseInt(process.env.${UPPER_NAME}_SYNC_INTERVAL ?? '3600', 10),
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.apiKey) {
    throw new Error('${UPPER_NAME}_API_KEY is required');
  }

  return config;
}
CONFIG
}

# =============================================================================
# Template: ts/src/client.ts
# =============================================================================

generate_ts_client() {
    write_file "${TARGET_DIR}/ts/src/client.ts" <<CLIENT
/**
 * ${PASCAL_NAME} API Client
 */

import { createLogger } from '@nself/plugin-utils';
import type { ${PASCAL_NAME}ApiRecord } from './types.js';

const logger = createLogger('${SHORT_NAME}:client');

export class ${PASCAL_NAME}Client {
  private readonly apiKey: string;
  private readonly baseUrl: string;

  constructor(apiKey: string, baseUrl = 'https://api.example.com/v1') {
    this.apiKey = apiKey;
    this.baseUrl = baseUrl;
    logger.info('${PASCAL_NAME} client initialized');
  }

  // =========================================================================
  // HTTP Helpers
  // =========================================================================

  private async request<T>(
    method: string,
    path: string,
    body?: Record<string, unknown>
  ): Promise<T> {
    const url = \`\${this.baseUrl}\${path}\`;
    const headers: Record<string, string> = {
      'Authorization': \`Bearer \${this.apiKey}\`,
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };

    const response = await fetch(url, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (!response.ok) {
      const text = await response.text().catch(() => '');
      throw new Error(
        \`${PASCAL_NAME} API error \${response.status} \${response.statusText}: \${text}\`
      );
    }

    return response.json() as Promise<T>;
  }

  private get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  // =========================================================================
  // Records
  // Adapt these methods to match the actual ${PASCAL_NAME} API
  // =========================================================================

  /**
   * List all records from ${PASCAL_NAME}
   */
  async listRecords(options: { limit?: number; after?: string } = {}): Promise<${PASCAL_NAME}ApiRecord[]> {
    logger.info('Fetching ${PLUGIN_NAME} records', options);

    // TODO: Replace with actual ${PASCAL_NAME} API endpoint and pagination logic
    const params = new URLSearchParams();
    if (options.limit) params.set('limit', String(options.limit));
    if (options.after) params.set('after', options.after);

    const query = params.toString() ? \`?\${params.toString()}\` : '';
    const data = await this.get<{ data: ${PASCAL_NAME}ApiRecord[] }>(\`/records\${query}\`);

    return data.data;
  }

  /**
   * Get a single record by ID
   */
  async getRecord(id: string): Promise<${PASCAL_NAME}ApiRecord> {
    logger.info('Fetching ${PLUGIN_NAME} record', { id });
    return this.get<${PASCAL_NAME}ApiRecord>(\`/records/\${id}\`);
  }

  /**
   * Test API connectivity and credentials
   */
  async testConnection(): Promise<boolean> {
    try {
      // TODO: Replace with a lightweight ${PASCAL_NAME} API health/ping endpoint
      await this.get('/');
      return true;
    } catch {
      return false;
    }
  }
}
CLIENT
}

# =============================================================================
# Template: ts/src/database.ts
# =============================================================================

generate_ts_database() {
    write_file "${TARGET_DIR}/ts/src/database.ts" <<DATABASE
/**
 * ${PASCAL_NAME} Database Operations
 * CRUD operations using PostgreSQL.
 * All tables use source_account_id for multi-app isolation.
 * Table prefix: ${TABLE_PREFIX}
 */

import { createDatabase, createLogger, normalizeSourceAccountId, type Database } from '@nself/plugin-utils';
import type { ${PASCAL_NAME}Record, ${PASCAL_NAME}WebhookEventRecord } from './types.js';

const logger = createLogger('${SHORT_NAME}:db');

export class ${PASCAL_NAME}Database {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ${PASCAL_NAME}Database {
    return new ${PASCAL_NAME}Database(this.db, sourceAccountId);
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  // =========================================================================
  // Records — ${TABLE_PREFIX}records
  // =========================================================================

  /**
   * Upsert a record (insert or update on conflict with external_id).
   * Always sets source_account_id from this instance.
   */
  async upsertRecord(record: Omit<${PASCAL_NAME}Record, 'id' | 'source_account_id' | 'synced_at'>): Promise<void> {
    logger.debug('Upserting record', { external_id: record.external_id });

    await this.db.query(
      \`INSERT INTO ${TABLE_PREFIX}records (
        source_account_id,
        external_id,
        name,
        description,
        status,
        data,
        created_at,
        updated_at,
        synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      ON CONFLICT (source_account_id, external_id)
      WHERE external_id IS NOT NULL
      DO UPDATE SET
        name        = EXCLUDED.name,
        description = EXCLUDED.description,
        status      = EXCLUDED.status,
        data        = EXCLUDED.data,
        updated_at  = EXCLUDED.updated_at,
        synced_at   = NOW()\`,
      [
        this.sourceAccountId,
        record.external_id,
        record.name,
        record.description,
        record.status,
        JSON.stringify(record.data),
        record.created_at,
        record.updated_at,
      ]
    );
  }

  /**
   * Get a single record by external_id, scoped to this source_account_id.
   */
  async getRecord(externalId: string): Promise<${PASCAL_NAME}Record | null> {
    const result = await this.db.query<${PASCAL_NAME}Record>(
      \`SELECT * FROM ${TABLE_PREFIX}records
       WHERE source_account_id = $1 AND external_id = $2
       LIMIT 1\`,
      [this.sourceAccountId, externalId]
    );
    return result.rows[0] ?? null;
  }

  /**
   * List records for this source_account_id.
   */
  async listRecords(limit = 100, offset = 0): Promise<${PASCAL_NAME}Record[]> {
    const result = await this.db.query<${PASCAL_NAME}Record>(
      \`SELECT * FROM ${TABLE_PREFIX}records
       WHERE source_account_id = $1
       ORDER BY updated_at DESC NULLS LAST
       LIMIT $2 OFFSET $3\`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  /**
   * Count all records for this source_account_id.
   */
  async countRecords(): Promise<number> {
    const result = await this.db.query<{ count: string }>(
      \`SELECT COUNT(*) AS count FROM ${TABLE_PREFIX}records
       WHERE source_account_id = $1\`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Webhook Events — ${TABLE_PREFIX}webhook_events
  // =========================================================================

  /**
   * Insert a new webhook event.
   */
  async insertWebhookEvent(
    event: Omit<${PASCAL_NAME}WebhookEventRecord, 'id' | 'source_account_id' | 'received_at'>
  ): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      \`INSERT INTO ${TABLE_PREFIX}webhook_events (
        source_account_id,
        event_type,
        action,
        external_id,
        data,
        processed,
        received_at
      ) VALUES ($1, $2, $3, $4, $5, FALSE, NOW())
      RETURNING id\`,
      [
        this.sourceAccountId,
        event.event_type,
        event.action,
        event.external_id,
        JSON.stringify(event.data),
      ]
    );
    return result.rows[0]!.id;
  }

  /**
   * Mark a webhook event as processed (or failed).
   */
  async markWebhookProcessed(id: string, error?: string): Promise<void> {
    await this.db.query(
      \`UPDATE ${TABLE_PREFIX}webhook_events
       SET processed = TRUE, processed_at = NOW(), error = $2
       WHERE id = $1\`,
      [id, error ?? null]
    );
  }

  /**
   * Fetch unprocessed webhook events for this source_account_id.
   */
  async getUnprocessedWebhooks(limit = 50): Promise<${PASCAL_NAME}WebhookEventRecord[]> {
    const result = await this.db.query<${PASCAL_NAME}WebhookEventRecord>(
      \`SELECT * FROM ${TABLE_PREFIX}webhook_events
       WHERE source_account_id = $1 AND processed = FALSE
       ORDER BY received_at ASC
       LIMIT $2\`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }
}
DATABASE
}

# =============================================================================
# Template: ts/src/sync.ts
# =============================================================================

generate_ts_sync() {
    write_file "${TARGET_DIR}/ts/src/sync.ts" <<SYNC
/**
 * ${PASCAL_NAME} Data Synchronization Service
 */

import { createLogger } from '@nself/plugin-utils';
import { ${PASCAL_NAME}Client } from './client.js';
import { ${PASCAL_NAME}Database } from './database.js';
import type { SyncOptions, SyncResult, SyncStats } from './types.js';

const logger = createLogger('${SHORT_NAME}:sync');

export class ${PASCAL_NAME}SyncService {
  private client: ${PASCAL_NAME}Client;
  private db: ${PASCAL_NAME}Database;
  private syncing = false;

  constructor(client: ${PASCAL_NAME}Client, db: ${PASCAL_NAME}Database) {
    this.client = client;
    this.db = db;
  }

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    if (this.syncing) {
      throw new Error('Sync already in progress');
    }

    this.syncing = true;
    const startTime = Date.now();
    const errors: string[] = [];
    const stats: SyncStats = {
      records: 0,
      lastSyncedAt: null,
    };

    try {
      logger.info('Starting ${PASCAL_NAME} sync', options);

      // -----------------------------------------------------------------------
      // Sync records
      // -----------------------------------------------------------------------
      try {
        const records = await this.client.listRecords({
          limit: options.limit,
        });

        for (const record of records) {
          await this.db.upsertRecord({
            external_id: record.id ?? null,
            name: (record.name as string) ?? null,
            description: (record.description as string) ?? null,
            status: (record.status as string) ?? null,
            data: record as Record<string, unknown>,
            created_at: record.created_at ? new Date(record.created_at as string) : null,
            updated_at: record.updated_at ? new Date(record.updated_at as string) : null,
          });
          stats.records++;
        }

        logger.info('Records synced', { count: stats.records });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        logger.error('Failed to sync records', { error: message });
        errors.push(\`records: \${message}\`);
      }

      stats.lastSyncedAt = new Date();

      const duration = Date.now() - startTime;
      logger.info('${PASCAL_NAME} sync complete', { stats, duration, errors: errors.length });

      return { success: errors.length === 0, stats, errors, duration };
    } finally {
      this.syncing = false;
    }
  }

  isSyncing(): boolean {
    return this.syncing;
  }
}
SYNC
}

# =============================================================================
# Template: ts/src/webhooks.ts
# =============================================================================

generate_ts_webhooks() {
    write_file "${TARGET_DIR}/ts/src/webhooks.ts" <<WEBHOOKS
/**
 * ${PASCAL_NAME} Webhook Handlers
 */

import { createLogger } from '@nself/plugin-utils';
import { ${PASCAL_NAME}Database } from './database.js';
import type { ${PASCAL_NAME}WebhookPayload } from './types.js';

const logger = createLogger('${SHORT_NAME}:webhooks');

export type WebhookHandlerFn = (payload: ${PASCAL_NAME}WebhookPayload) => Promise<void>;

export class ${PASCAL_NAME}WebhookHandler {
  private db: ${PASCAL_NAME}Database;
  private handlers: Map<string, WebhookHandlerFn>;

  constructor(db: ${PASCAL_NAME}Database) {
    this.db = db;
    this.handlers = new Map();
    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    this.register('created', this.handleCreated.bind(this));
    this.register('updated', this.handleUpdated.bind(this));
    this.register('deleted', this.handleDeleted.bind(this));
  }

  register(event: string, handler: WebhookHandlerFn): void {
    this.handlers.set(event, handler);
    logger.debug('Registered handler', { event });
  }

  async handleEvent(payload: ${PASCAL_NAME}WebhookPayload): Promise<void> {
    const { event, action } = payload;
    logger.info('Received webhook event', { event, action });

    // Store the event before processing
    const eventId = await this.db.insertWebhookEvent({
      event_type: event,
      action: action ?? null,
      external_id: (payload.id as string) ?? null,
      data: payload.data,
      processed: false,
      processed_at: null,
      error: null,
    });

    // Route to handler
    const handlerKey = action ?? event;
    const handler = this.handlers.get(handlerKey) ?? this.handlers.get(event);

    if (!handler) {
      logger.warn('No handler for event', { event, action });
      await this.db.markWebhookProcessed(eventId, \`No handler for event: \${event}\`);
      return;
    }

    try {
      await handler(payload);
      await this.db.markWebhookProcessed(eventId);
      logger.info('Webhook event processed', { event, action });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      logger.error('Webhook handler failed', { event, action, error: message });
      await this.db.markWebhookProcessed(eventId, message);
      throw err;
    }
  }

  // =========================================================================
  // Default Handlers — replace with actual ${PASCAL_NAME} logic
  // =========================================================================

  private async handleCreated(payload: ${PASCAL_NAME}WebhookPayload): Promise<void> {
    logger.info('Handling created event', { id: payload.id });
    // TODO: Upsert the created resource into the database
    // const record = payload.data as ${PASCAL_NAME}ApiRecord;
    // await this.db.upsertRecord({ ... });
  }

  private async handleUpdated(payload: ${PASCAL_NAME}WebhookPayload): Promise<void> {
    logger.info('Handling updated event', { id: payload.id });
    // TODO: Update the resource in the database
  }

  private async handleDeleted(payload: ${PASCAL_NAME}WebhookPayload): Promise<void> {
    logger.info('Handling deleted event', { id: payload.id });
    // TODO: Mark the resource as deleted or remove it
  }
}
WEBHOOKS
}

# =============================================================================
# Template: ts/src/server.ts
# =============================================================================

generate_ts_server() {
    write_file "${TARGET_DIR}/ts/src/server.ts" <<SERVER
/**
 * ${PASCAL_NAME} Plugin Server
 * HTTP server for webhooks and REST API endpoints (Fastify)
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  ApiRateLimiter,
  createAuthHook,
  createRateLimitHook,
} from '@nself/plugin-utils';
import { ${PASCAL_NAME}Client } from './client.js';
import { ${PASCAL_NAME}Database } from './database.js';
import { ${PASCAL_NAME}SyncService } from './sync.js';
import { ${PASCAL_NAME}WebhookHandler } from './webhooks.js';
import { loadConfig, type Config } from './config.js';
import type { ${PASCAL_NAME}WebhookPayload } from './types.js';

const logger = createLogger('${SHORT_NAME}:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const client = new ${PASCAL_NAME}Client(fullConfig.apiKey);
  const db = new ${PASCAL_NAME}Database();
  const syncService = new ${PASCAL_NAME}SyncService(client, db);
  const webhookHandler = new ${PASCAL_NAME}WebhookHandler(db);

  // Connect to database
  await db.connect();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10 MB
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Rate limiting
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Optional API key auth
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => ({ status: 'ok', plugin: '${PLUGIN_NAME}' }));

  app.get('/status', async () => {
    const recordCount = await db.countRecords();
    return {
      plugin: '${PLUGIN_NAME}',
      syncing: syncService.isSyncing(),
      records: recordCount,
    };
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post<{ Body: ${PASCAL_NAME}WebhookPayload }>(
    '/webhooks/${PLUGIN_NAME}',
    {
      config: { skipAuth: true },
    },
    async (request, reply) => {
      try {
        // TODO: Add signature verification here if ${PASCAL_NAME} supports it
        // verifySignature(request.rawBody, request.headers['x-${PLUGIN_NAME}-signature'], fullConfig.webhookSecret);

        await webhookHandler.handleEvent(request.body);
        return reply.code(200).send({ received: true });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        logger.error('Webhook processing failed', { error: message });
        return reply.code(500).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Sync Endpoint
  // =========================================================================

  app.post('/sync', async (_request, reply) => {
    if (syncService.isSyncing()) {
      return reply.code(409).send({ error: 'Sync already in progress' });
    }

    // Run async — return immediately
    syncService.sync().catch(err => {
      logger.error('Background sync failed', { error: err instanceof Error ? err.message : String(err) });
    });

    return reply.code(202).send({ message: 'Sync started' });
  });

  // =========================================================================
  // Records API
  // =========================================================================

  app.get('/records', async (request) => {
    const query = request.query as { limit?: string; offset?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);
    const records = await db.listRecords(limit, offset);
    return { data: records, limit, offset };
  });

  return { app, db, syncService };
}

// =============================================================================
// Entry Point (when run directly: node dist/server.js)
// =============================================================================

const config = loadConfig();

createServer(config).then(async ({ app }) => {
  await app.listen({ port: config.port, host: config.host });
  logger.info(\`${PASCAL_NAME} plugin server listening\`, { port: config.port });
}).catch(err => {
  logger.error('Failed to start server', { error: err instanceof Error ? err.message : String(err) });
  process.exit(1);
});
SERVER
}

# =============================================================================
# Template: ts/src/cli.ts
# =============================================================================

generate_ts_cli() {
    write_file "${TARGET_DIR}/ts/src/cli.ts" <<CLI
#!/usr/bin/env node
/**
 * ${PASCAL_NAME} Plugin CLI
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ${PASCAL_NAME}Client } from './client.js';
import { ${PASCAL_NAME}Database } from './database.js';
import { ${PASCAL_NAME}SyncService } from './sync.js';
import { createServer } from './server.js';

const logger = createLogger('${SHORT_NAME}:cli');
const program = new Command();

program
  .name('nself-${PLUGIN_NAME}')
  .description('${DESCRIPTION}')
  .version('1.0.0');

// ---------------------------------------------------------------------------
// sync command
// ---------------------------------------------------------------------------

program
  .command('sync')
  .description('Sync ${PASCAL_NAME} data to database')
  .option('--since <date>', 'Only sync changes since date (ISO 8601)')
  .option('--limit <n>', 'Maximum records to sync', parseInt)
  .action(async (options: { since?: string; limit?: number }) => {
    try {
      const config = loadConfig();
      const client = new ${PASCAL_NAME}Client(config.apiKey);
      const db = new ${PASCAL_NAME}Database();
      await db.connect();

      const syncService = new ${PASCAL_NAME}SyncService(client, db);
      const result = await syncService.sync({
        since: options.since ? new Date(options.since) : undefined,
        limit: options.limit,
      });

      console.log('\nSync Results:');
      console.log('=============');
      console.log('Records:  ', result.stats.records);
      console.log('Duration: ', \`\${(result.duration / 1000).toFixed(1)}s\`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        result.errors.forEach(e => console.log(' -', e));
        process.exitCode = 1;
      }

      await db.disconnect();
    } catch (err) {
      logger.error('Sync failed', { error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

// ---------------------------------------------------------------------------
// server command
// ---------------------------------------------------------------------------

program
  .command('server')
  .description('Start the ${PASCAL_NAME} webhook server')
  .action(async () => {
    try {
      const config = loadConfig();
      const { app } = await createServer(config);
      await app.listen({ port: config.port, host: config.host });
      console.log(\`${PASCAL_NAME} server listening on \${config.host}:\${config.port}\`);
    } catch (err) {
      logger.error('Server failed to start', { error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

// ---------------------------------------------------------------------------
// status command
// ---------------------------------------------------------------------------

program
  .command('status')
  .description('Show ${PASCAL_NAME} plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new ${PASCAL_NAME}Database();
      await db.connect();

      const recordCount = await db.countRecords();
      const unprocessed = await db.getUnprocessedWebhooks(1);

      console.log('\n${PASCAL_NAME} Plugin Status');
      console.log('=======================');
      console.log('Records:             ', recordCount);
      console.log('Unprocessed webhooks:', unprocessed.length > 0 ? '>0 (run sync)' : '0');

      await db.disconnect();
    } catch (err) {
      logger.error('Status check failed', { error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program.parse(process.argv);
CLI
    chmod +x "${TARGET_DIR}/ts/src/cli.ts"
}

# =============================================================================
# Template: ts/src/index.ts
# =============================================================================

generate_ts_index() {
    write_file "${TARGET_DIR}/ts/src/index.ts" <<TSINDEX
/**
 * ${PASCAL_NAME} Plugin for nself
 */

export { ${PASCAL_NAME}Client } from './client.js';
export { ${PASCAL_NAME}Database } from './database.js';
export { ${PASCAL_NAME}SyncService } from './sync.js';
export { ${PASCAL_NAME}WebhookHandler } from './webhooks.js';
export { createServer } from './server.js';
export { loadConfig } from './config.js';
export * from './types.js';
TSINDEX
}

# =============================================================================
# Template: webhooks/README.md
# =============================================================================

generate_webhooks_readme() {
    write_file "${TARGET_DIR}/webhooks/README.md" <<WEBHOOKSREADME
# ${PASCAL_NAME} Plugin Webhooks

## Webhook Endpoint

\`\`\`
POST /webhooks/${PLUGIN_NAME}
\`\`\`

## Supported Events

| Event | Action | Description |
|-------|--------|-------------|
| \`created\` | — | A resource was created |
| \`updated\` | — | A resource was updated |
| \`deleted\` | — | A resource was deleted |

## Signature Verification

Set \`${UPPER_NAME}_WEBHOOK_SECRET\` to enable HMAC signature verification.

## Configuring Your Webhook

In your ${PASCAL_NAME} dashboard, point the webhook URL to:

\`\`\`
https://your-nself-domain.com/webhooks/${PLUGIN_NAME}
\`\`\`

Content-Type: \`application/json\`

## Testing Locally

Use \`ngrok\` or similar tunneling to expose your local server:

\`\`\`bash
ngrok http 3000
# Then set webhook URL to: https://<ngrok-id>.ngrok.io/webhooks/${PLUGIN_NAME}
\`\`\`

## Handler Script

The Bash webhook handler is at \`webhooks/handler.sh\`.
It receives the raw payload via stdin and routes to event-specific scripts.
WEBHOOKSREADME
}

# =============================================================================
# Registry Entry — printed to stdout for manual addition to registry.json
# =============================================================================

print_registry_entry() {
    printf "\n${BOLD}${CYAN}Registry Entry Template${RESET}\n"
    printf "${CYAN}========================${RESET}\n"
    printf "Add the following entry to ${PLUGINS_ROOT}/registry.json under \"plugins\":\n\n"

    cat <<REGISTRY
  "${PLUGIN_NAME}": {
    "name": "${PLUGIN_NAME}",
    "version": "1.0.0",
    "description": "${DESCRIPTION}",
    "author": "${AUTHOR}",
    "license": "MIT",
    "homepage": "https://github.com/nself-org/plugins/tree/main/free/${PLUGIN_NAME}",
    "repository": "https://github.com/nself-org/plugins",
    "path": "free/${PLUGIN_NAME}",
    "minNselfVersion": "0.4.8",
    "category": "${CATEGORY}",
    "tags": [
      "${PLUGIN_NAME}"
    ],
    "implementation": {
      "language": "typescript",
      "runtime": "node",
      "minNodeVersion": "18.0.0",
      "entryPoint": "ts/dist/index.js",
      "cli": "ts/dist/cli.js",
      "cliName": "nself-${PLUGIN_NAME}",
      "defaultPort": 3000,
      "packageManager": "pnpm",
      "framework": "fastify"
    },
    "tables": [
      "${TABLE_PREFIX}records",
      "${TABLE_PREFIX}webhook_events"
    ],
    "webhooks": [
      "created",
      "updated",
      "deleted"
    ],
    "cliCommands": [
      { "name": "sync",   "description": "Sync ${PASCAL_NAME} data to database" },
      { "name": "server", "description": "Start the webhook server" },
      { "name": "status", "description": "Show plugin status" }
    ],
    "multiApp": {
      "supported": true,
      "isolationColumn": "source_account_id",
      "pkStrategy": "uuid",
      "defaultValue": "primary"
    },
    "envVars": {
      "required": ["DATABASE_URL", "${UPPER_NAME}_API_KEY"],
      "optional": ["${UPPER_NAME}_WEBHOOK_SECRET", "${UPPER_NAME}_PLUGIN_PORT"]
    }
  }
REGISTRY
}

# =============================================================================
# Main Execution
# =============================================================================

main() {
    # Check for --help before parsing fully
    case "${1:-}" in
        --help | -h | "")
            if [ "${1:-}" != "" ]; then
                usage
            else
                usage
            fi
            exit 0
            ;;
    esac

    parse_args "$@"

    # Validate inputs
    validate_plugin_name "$PLUGIN_NAME" || exit 1
    validate_category "$CATEGORY" || exit 1

    # Compute derived names
    compute_names

    # Set up target directory path
    setup_target_dir

    # Display summary
    log_header "nself Plugin Generator"
    printf "\n"
    log_step "Plugin"   "$PLUGIN_NAME"
    log_step "Category" "$CATEGORY"
    log_step "PascalCase" "$PASCAL_NAME"
    log_step "TablePrefix" "${TABLE_PREFIX}*"
    log_step "Author"   "$AUTHOR"
    log_step "Description" "$DESCRIPTION"
    log_step "Target"   "$TARGET_DIR"
    printf "\n"

    # Create directory scaffold
    log_header "Creating Directory Structure"
    make_dir "$TARGET_DIR"
    make_dir "${TARGET_DIR}/schema"
    make_dir "${TARGET_DIR}/templates"
    make_dir "${TARGET_DIR}/ts"
    make_dir "${TARGET_DIR}/ts/src"
    make_dir "${TARGET_DIR}/webhooks"

    # Generate files
    log_header "Generating Files"

    generate_readme
    generate_plugin_json
    generate_install_sh
    generate_uninstall_sh
    generate_schema_sql
    generate_env_template
    generate_ts_package_json
    generate_ts_tsconfig
    generate_ts_types
    generate_ts_config
    generate_ts_client
    generate_ts_database
    generate_ts_sync
    generate_ts_webhooks
    generate_ts_server
    generate_ts_cli
    generate_ts_index
    generate_webhooks_readme

    # Summary
    log_header "Done"
    printf "\n"
    printf "${GREEN}${BOLD}Plugin scaffold created at:${RESET}\n"
    printf "  %s\n\n" "$TARGET_DIR"

    printf "${BOLD}Next steps:${RESET}\n"
    printf "  1. ${YELLOW}Review and adapt${RESET} ts/src/client.ts to the real ${PASCAL_NAME} API\n"
    printf "  2. ${YELLOW}Expand${RESET} ts/src/types.ts with actual API response shapes\n"
    printf "  3. ${YELLOW}Expand${RESET} schema/${PLUGIN_NAME}.sql with all required tables\n"
    printf "  4. ${YELLOW}Implement${RESET} webhook handlers in ts/src/webhooks.ts\n"
    printf "  5. ${YELLOW}Install deps${RESET}: cd %s/ts && pnpm install\n" "$TARGET_DIR"
    printf "  6. ${YELLOW}Build${RESET}: pnpm run build\n"
    printf "  7. ${YELLOW}Add to registry${RESET}: see template below\n\n"

    print_registry_entry

    printf "\n"
}

main "$@"
