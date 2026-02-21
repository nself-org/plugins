#!/usr/bin/env bash
# deploy.sh - Deploy nself plugin registry to Cloudflare Workers
#
# This script handles the complete deployment of the plugin registry Worker.
# It can be run manually or triggered by GitHub Actions.
#
# Prerequisites:
#   - Node.js 18+
#   - Cloudflare account with Workers enabled
#   - nself.org zone configured in Cloudflare
#
# Usage:
#   ./deploy.sh              # Interactive setup and deploy
#   ./deploy.sh --production # Deploy to production
#   ./deploy.sh --dev        # Deploy to dev (workers.dev)
#   ./deploy.sh --setup      # First-time setup only

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { printf "${BLUE}[INFO]${NC} %s\n" "$1"; }
log_success() { printf "${GREEN}[SUCCESS]${NC} %s\n" "$1"; }
log_warning() { printf "${YELLOW}[WARNING]${NC} %s\n" "$1"; }
log_error() { printf "${RED}[ERROR]${NC} %s\n" "$1" >&2; }

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Node.js
    if ! command -v node >/dev/null 2>&1; then
        log_error "Node.js is required. Install from https://nodejs.org"
        exit 1
    fi

    local node_version
    node_version=$(node --version | sed 's/v//' | cut -d. -f1)
    if (( node_version < 18 )); then
        log_error "Node.js 18+ required. Current: $(node --version)"
        exit 1
    fi

    log_success "Node.js $(node --version) found"

    # npm
    if ! command -v npm >/dev/null 2>&1; then
        log_error "npm is required"
        exit 1
    fi

    log_success "npm $(npm --version) found"
}

# Install dependencies
install_dependencies() {
    log_info "Installing dependencies..."

    if [[ ! -d "node_modules" ]] || [[ ! -f "node_modules/.package-lock.json" ]]; then
        npm install
    else
        log_info "Dependencies already installed"
    fi

    # Ensure wrangler is available
    if ! npx wrangler --version >/dev/null 2>&1; then
        log_error "Failed to install wrangler"
        exit 1
    fi

    log_success "Wrangler $(npx wrangler --version) ready"
}

# Authenticate with Cloudflare
authenticate() {
    log_info "Checking Cloudflare authentication..."

    # Check if already logged in
    if npx wrangler whoami 2>/dev/null | grep -q "You are logged in"; then
        log_success "Already authenticated with Cloudflare"
        return 0
    fi

    # Check for API token in environment
    if [[ -n "${CLOUDFLARE_API_TOKEN:-}" ]]; then
        log_success "Using CLOUDFLARE_API_TOKEN from environment"
        return 0
    fi

    # Check for Global API Key in environment
    if [[ -n "${CLOUDFLARE_API_KEY:-}" ]] && [[ -n "${CLOUDFLARE_EMAIL:-}" ]]; then
        log_success "Using CLOUDFLARE_API_KEY from environment"
        return 0
    fi

    # Check for local credentials file (create your own .env file with credentials)
    local creds_file="$SCRIPT_DIR/.env"
    if [[ -f "$creds_file" ]]; then
        log_info "Found .env file, loading Cloudflare credentials..."
        # shellcheck source=/dev/null
        source "$creds_file"

        if [[ -n "${CLOUDFLARE_API_KEY:-}" ]] && [[ -n "${CLOUDFLARE_EMAIL:-}" ]]; then
            export CLOUDFLARE_API_KEY
            export CLOUDFLARE_EMAIL
            log_success "Loaded Cloudflare credentials from .env"
            return 0
        fi
    fi

    # Interactive login
    log_info "Please authenticate with Cloudflare..."
    npx wrangler login
}

# Create KV namespace
create_kv_namespace() {
    log_info "Checking KV namespace..."

    # Check if namespace exists
    local namespaces
    namespaces=$(npx wrangler kv:namespace list 2>/dev/null || echo "[]")

    if printf '%s' "$namespaces" | grep -q "nself-plugin-registry-PLUGINS_KV"; then
        log_success "KV namespace already exists"

        # Extract and update ID if needed
        local namespace_id
        namespace_id=$(printf '%s' "$namespaces" | grep -A2 "nself-plugin-registry-PLUGINS_KV" | grep '"id"' | cut -d'"' -f4)

        if [[ -n "$namespace_id" ]] && grep -q 'id = ""' wrangler.toml; then
            log_info "Updating wrangler.toml with existing namespace ID..."
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s/id = \"\"/id = \"$namespace_id\"/" wrangler.toml
            else
                sed -i "s/id = \"\"/id = \"$namespace_id\"/" wrangler.toml
            fi
            log_success "Updated wrangler.toml with namespace ID: $namespace_id"
        fi
        return 0
    fi

    log_info "Creating KV namespace..."
    local output
    output=$(npx wrangler kv:namespace create "PLUGINS_KV" 2>&1)

    # Extract namespace ID
    local namespace_id
    namespace_id=$(printf '%s' "$output" | grep -o 'id = "[^"]*"' | cut -d'"' -f2)

    if [[ -n "$namespace_id" ]]; then
        log_success "Created KV namespace: $namespace_id"

        # Update wrangler.toml
        if grep -q 'id = ""' wrangler.toml; then
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s/id = \"\"/id = \"$namespace_id\"/" wrangler.toml
            else
                sed -i "s/id = \"\"/id = \"$namespace_id\"/" wrangler.toml
            fi
            log_success "Updated wrangler.toml with namespace ID"
        fi
    else
        log_warning "Could not extract namespace ID. Please update wrangler.toml manually."
        printf '%s\n' "$output"
    fi
}

# Set secrets
set_secrets() {
    log_info "Checking secrets..."

    # Check if GITHUB_SYNC_TOKEN secret exists
    # Note: There's no direct way to check if a secret exists, so we just set it

    # Generate a random sync token if not provided
    local sync_token="${GITHUB_SYNC_TOKEN:-}"
    if [[ -z "$sync_token" ]]; then
        sync_token=$(openssl rand -hex 32 2>/dev/null || head -c 64 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 64)
    fi

    log_info "Setting GITHUB_SYNC_TOKEN..."
    printf '%s' "$sync_token" | npx wrangler secret put GITHUB_SYNC_TOKEN 2>/dev/null || {
        log_warning "Could not set secret automatically. Run manually:"
        printf "  echo 'YOUR_TOKEN' | npx wrangler secret put GITHUB_SYNC_TOKEN\n"
    }

    log_success "Secrets configured"

    if [[ -z "${GITHUB_SYNC_TOKEN:-}" ]]; then
        log_info "IMPORTANT: Save this sync token for GitHub Actions secrets:"
        printf "\n  CLOUDFLARE_WORKER_SYNC_TOKEN=%s\n\n" "$sync_token"
        printf "Add this to GitHub repo secrets: Settings > Secrets > Actions\n\n"
    fi
}

# Deploy worker
deploy_worker() {
    local env="${1:-production}"

    log_info "Deploying to $env..."

    if [[ "$env" == "production" ]]; then
        npx wrangler deploy --env production
    else
        npx wrangler deploy
    fi

    log_success "Deployed successfully!"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."

    local url="https://plugins.nself.org/health"
    local response

    # Wait a moment for deployment to propagate
    sleep 3

    response=$(curl -sf "$url" 2>/dev/null || echo "")

    if printf '%s' "$response" | grep -q '"status":"healthy"'; then
        log_success "Worker is healthy!"
        printf "  Response: %s\n" "$response"
    else
        log_warning "Could not verify worker health. It may take a few minutes to propagate."
        log_info "Try: curl $url"
    fi
}

# Show help
show_help() {
    printf "Usage: %s [OPTIONS]\n\n" "$0"
    printf "Options:\n"
    printf "  --production, -p  Deploy to production (plugins.nself.org)\n"
    printf "  --dev, -d         Deploy to dev (workers.dev subdomain)\n"
    printf "  --setup, -s       First-time setup only (create KV, set secrets)\n"
    printf "  --help, -h        Show this help\n"
    printf "\nEnvironment Variables:\n"
    printf "  CLOUDFLARE_API_TOKEN    Cloudflare API token\n"
    printf "  CLOUDFLARE_API_KEY      Cloudflare Global API key\n"
    printf "  CLOUDFLARE_EMAIL        Cloudflare account email\n"
    printf "  GITHUB_SYNC_TOKEN       Token for webhook authentication\n"
}

# Main
main() {
    local deploy_env="production"
    local setup_only=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --production|-p)
                deploy_env="production"
                shift
                ;;
            --dev|-d)
                deploy_env="dev"
                shift
                ;;
            --setup|-s)
                setup_only=true
                shift
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                shift
                ;;
        esac
    done

    printf "\n${BLUE}═══════════════════════════════════════════════════════════${NC}\n"
    printf "${BLUE}  nself Plugin Registry - Cloudflare Worker${NC}\n"
    printf "${BLUE}═══════════════════════════════════════════════════════════${NC}\n\n"

    check_prerequisites
    install_dependencies
    authenticate
    create_kv_namespace

    if [[ "$setup_only" == true ]]; then
        set_secrets
        printf "\n${GREEN}Setup complete!${NC}\n"
        printf "Run './deploy.sh --production' to deploy.\n\n"
        exit 0
    fi

    deploy_worker "$deploy_env"
    verify_deployment

    printf "\n${GREEN}═══════════════════════════════════════════════════════════${NC}\n"
    printf "${GREEN}  Deployment Complete!${NC}\n"
    printf "${GREEN}═══════════════════════════════════════════════════════════${NC}\n\n"

    printf "Endpoints:\n"
    printf "  Registry:  https://plugins.nself.org/registry.json\n"
    printf "  Health:    https://plugins.nself.org/health\n"
    printf "  Stats:     https://plugins.nself.org/stats\n"
    printf "\n"
}

main "$@"
