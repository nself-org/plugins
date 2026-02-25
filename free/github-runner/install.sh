#!/bin/bash
# =============================================================================
# GitHub Actions Runner Plugin Installer
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$(dirname "$PLUGIN_DIR")")/shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

RUNNER_DIR="${HOME}/.nself/runners/github-runner"
LOG_DIR="${HOME}/.nself/logs/plugins/github-runner"
PID_FILE="${LOG_DIR}/runner.pid"

# =============================================================================
# Helpers
# =============================================================================

detect_arch() {
  local arch
  arch=$(uname -m)
  case "$arch" in
    x86_64)  echo "x64" ;;
    aarch64|arm64) echo "arm64" ;;
    armv7l)  echo "arm" ;;
    *)
      plugin_error "Unsupported architecture: $arch"
      exit 1
      ;;
  esac
}

detect_os() {
  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux)  echo "linux" ;;
    darwin) echo "osx" ;;
    *)
      plugin_error "Unsupported OS: $os"
      exit 1
      ;;
  esac
}

get_latest_runner_version() {
  local version
  version=$(curl -sSf \
    -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/actions/runner/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"v\([^"]*\)".*/\1/')

  if [[ -z "$version" ]]; then
    plugin_warn "Could not detect latest runner version, using 2.321.0"
    echo "2.321.0"
  else
    echo "$version"
  fi
}

get_registration_token() {
  local scope="${GITHUB_RUNNER_SCOPE:-org}"
  local token

  if [[ "$scope" == "org" ]]; then
    token=$(curl -sSf -X POST \
      -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/orgs/${GITHUB_RUNNER_ORG}/actions/runners/registration-token" \
      | grep '"token"' | head -1 | sed 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

    if [[ -z "$token" || "$token" == "null" ]]; then
      plugin_error "Failed to get registration token. Verify GITHUB_RUNNER_PAT has 'admin:org' scope."
      exit 1
    fi
    echo "$token"
  else
    local repo="${GITHUB_RUNNER_REPO:?GITHUB_RUNNER_REPO required for repo-scope runner}"
    token=$(curl -sSf -X POST \
      -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${GITHUB_RUNNER_ORG}/${repo}/actions/runners/registration-token" \
      | grep '"token"' | head -1 | sed 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

    if [[ -z "$token" || "$token" == "null" ]]; then
      plugin_error "Failed to get registration token for repo ${GITHUB_RUNNER_ORG}/${repo}."
      exit 1
    fi
    echo "$token"
  fi
}

config_url() {
  local scope="${GITHUB_RUNNER_SCOPE:-org}"
  if [[ "$scope" == "org" ]]; then
    echo "https://github.com/${GITHUB_RUNNER_ORG}"
  else
    echo "https://github.com/${GITHUB_RUNNER_ORG}/${GITHUB_RUNNER_REPO}"
  fi
}

# =============================================================================
# Installation
# =============================================================================

install_github_runner() {
  plugin_info "Installing GitHub Actions Runner plugin..."

  # ------------------------------------------------------------------
  # 1. Check required env vars
  # ------------------------------------------------------------------
  if ! plugin_check_env "github-runner" "GITHUB_RUNNER_PAT" "GITHUB_RUNNER_ORG"; then
    plugin_error "Required env vars missing. Set them in your .env file:"
    printf "  GITHUB_RUNNER_PAT=<GitHub PAT with admin:org scope>\n"
    printf "  GITHUB_RUNNER_ORG=<your-org-name>\n"
    exit 1
  fi

  # ------------------------------------------------------------------
  # 2. Verify PAT has required scope
  # ------------------------------------------------------------------
  plugin_info "Verifying GitHub PAT scope..."
  local scope_header
  scope_header=$(curl -sI \
    -H "Authorization: token ${GITHUB_RUNNER_PAT}" \
    "https://api.github.com/user" \
    | grep -i "x-oauth-scopes:" | tr -d '\r' | sed 's/.*: //')

  if [[ -z "$scope_header" ]]; then
    plugin_warn "Could not read PAT scopes. Continuing — install will fail if PAT lacks 'admin:org'."
  elif [[ "$scope_header" != *"admin:org"* && "$scope_header" != *"repo"* ]]; then
    plugin_error "PAT is missing required scope."
    printf "  Current scopes: %s\n" "$scope_header"
    printf "  Required: 'admin:org' (for org-level runner) or 'repo' (for repo-level)\n"
    printf "  Create a new PAT at: https://github.com/settings/tokens\n"
    exit 1
  else
    plugin_success "PAT scope verified: $scope_header"
  fi

  # ------------------------------------------------------------------
  # 3. Create directories
  # ------------------------------------------------------------------
  mkdir -p "$RUNNER_DIR" "$LOG_DIR"

  # ------------------------------------------------------------------
  # 4. Download runner binary
  # ------------------------------------------------------------------
  local os arch version
  os=$(detect_os)
  arch=$(detect_arch)
  version="${GITHUB_RUNNER_VERSION:-$(get_latest_runner_version)}"

  local runner_pkg="actions-runner-${os}-${arch}-${version}.tar.gz"
  local runner_url="https://github.com/actions/runner/releases/download/v${version}/${runner_pkg}"

  if [[ -f "${RUNNER_DIR}/run.sh" ]]; then
    plugin_info "Runner binary already present, skipping download."
  else
    plugin_info "Downloading GitHub Actions runner v${version} (${os}/${arch})..."
    curl -sSfL "$runner_url" -o "/tmp/${runner_pkg}"

    plugin_info "Extracting runner..."
    tar xzf "/tmp/${runner_pkg}" -C "$RUNNER_DIR"
    rm -f "/tmp/${runner_pkg}"
    plugin_success "Runner downloaded and extracted."
  fi

  # ------------------------------------------------------------------
  # 5. Configure runner
  # ------------------------------------------------------------------
  plugin_info "Configuring runner..."
  local reg_token
  reg_token=$(get_registration_token)

  local runner_name="${GITHUB_RUNNER_NAME:-$(hostname)-nself}"
  local runner_labels="${GITHUB_RUNNER_LABELS:-self-hosted,linux,x64,ubuntu-latest}"
  local runner_config_url
  runner_config_url=$(config_url)

  local group_flag=""
  [[ -n "${GITHUB_RUNNER_GROUP:-}" ]] && group_flag="--runnergroup ${GITHUB_RUNNER_GROUP}"

  (
    cd "$RUNNER_DIR"
    ./config.sh \
      --url "$runner_config_url" \
      --token "$reg_token" \
      --name "$runner_name" \
      --labels "$runner_labels" \
      $group_flag \
      --unattended \
      --replace
  )
  plugin_success "Runner configured: $runner_name"
  plugin_success "Labels: $runner_labels"

  # ------------------------------------------------------------------
  # 6. Install as service or start via nohup
  # ------------------------------------------------------------------
  _start_runner

  plugin_success "GitHub Actions Runner plugin installed successfully!"
  printf "\n"
  printf "Runner: %s\n" "$runner_name"
  printf "Org:    https://github.com/%s\n" "${GITHUB_RUNNER_ORG}"
  printf "Labels: %s\n" "$runner_labels"
  printf "\n"
  printf "Manage with:\n"
  printf "  nself plugin github-runner status\n"
  printf "  nself plugin github-runner logs\n"
  printf "  nself plugin github-runner stop\n"
  printf "  nself plugin github-runner start\n"
  printf "\n"
  printf "View runners in GitHub:\n"
  printf "  https://github.com/organizations/%s/settings/actions/runners\n" "${GITHUB_RUNNER_ORG}"
  printf "\n"
}

# Start the runner (systemd if available, else nohup)
_start_runner() {
  if command -v systemctl &>/dev/null && systemctl is-system-running --quiet 2>/dev/null; then
    plugin_info "Installing systemd service..."
    (
      cd "$RUNNER_DIR"
      if [[ $EUID -eq 0 ]]; then
        ./svc.sh install
        ./svc.sh start
      else
        sudo ./svc.sh install
        sudo ./svc.sh start
      fi
    ) && {
      plugin_success "Runner installed as systemd service."
      return 0
    } || {
      plugin_warn "systemd install failed, falling back to nohup."
    }
  fi

  # Fallback: nohup
  plugin_info "Starting runner (nohup)..."
  (
    cd "$RUNNER_DIR"
    nohup ./run.sh >> "${LOG_DIR}/runner.log" 2>&1 &
    echo $! > "$PID_FILE"
  )
  plugin_success "Runner started (PID: $(cat "$PID_FILE"))"
}

# Run installation
install_github_runner
