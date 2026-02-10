#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

chmod +x .github/hooks/pre-commit
chmod +x .github/hooks/commit-msg
chmod +x .github/scripts/no-attribution-check.sh

git config core.hooksPath .github/hooks

echo "Installed local git hooks from .github/hooks"
echo "Configured: core.hooksPath=.github/hooks"
