#!/usr/bin/env bash
# test-lint-compose-password-urls.sh
#
# Purpose: regression cases for scripts/lint-compose-password-urls.sh (D-0235).
#   Each case writes a one-service fragment, runs the lint on it and checks the
#   verdict. The reject cases are the bypasses the P7-PROD-33 review found (two
#   URLs on one line, password in the query, URL split across two to four
#   lines, secret names outside PASSWORD|PASSWD) plus the classic raw form;
#   the accept cases are the nested form and secret variables outside any
#   URL, a multi-line shell script included.
# Inputs: none. Fixtures are generated into a temp dir at run time and hold
#   variable references only, never a value.
# Outputs: "ok:"/"FAIL:" per case, final summary; exit 1 on any failure.
# Constraints: bash 3.2 compatible; no docker, no network.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
lint="$here/../../lint-compose-password-urls.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/lint-pw-urls.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
fail=0

# check NAME WANT(reject|accept) LINE... : fragment = a service env block of LINEs.
check() {
  name="$1" want="$2"; shift 2
  f="$tmp/$name.yml"
  printf 'services:\n  a:\n    environment:\n' > "$f"
  for l in "$@"; do printf '%s\n' "$l" >> "$f"; done
  if bash "$lint" "$f" >/dev/null 2>&1; then got=accept; else got=reject; fi
  if [ "$got" = "$want" ]; then echo "ok: $name ($got)"; else echo "FAIL: $name: want $want, got $got" >&2; fail=1; fi
}

check classic-raw reject '      - DATABASE_URL=postgresql://u:${POSTGRES_PASSWORD}@postgres/db'
check two-urls-one-line reject '      - X=http://a@b y://u:${POSTGRES_PASSWORD}@h/db'
check password-in-query reject '      DATABASE_URL: postgresql://postgres:5432/db?user=u&password=${POSTGRES_PASSWORD}'
check split-userinfo-head reject '      DATABASE_URL: >-' \
  '        postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}' '        @postgres:5432/db'
check split-userinfo-tail reject '      DATABASE_URL: >-' \
  '        postgresql://${POSTGRES_USER}:' '        ${POSTGRES_PASSWORD}@postgres:5432/db'
check split-three-lines reject '      D: >-' '        postgresql://u:' \
  '        ${POSTGRES_PASSWORD}' '        @pg/db'
check split-four-lines reject '      D: >-' '        postgresql://' '        u:' \
  '        ${POSTGRES_PASSWORD}' '        @pg/db'
check name-pass reject '      - A=postgresql://u:${DB_PASS}@postgres/db'
check name-pwd reject '      - E=postgresql://u:${DB_PWD}@postgres/db'
check name-secret reject '      - B=amqp://u:${RABBIT_SECRET}@mq:5672'
check name-token-unbraced reject '      - C=redis://:$REDIS_AUTH_TOKEN@redis:6379'
check name-lowercase-default reject '      - D=postgres://u:${db_pass:-x}@pg/db'
check nested-form accept \
  '      - DATABASE_URL=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/${POSTGRES_DB}' \
  '      - REDIS_URL=redis://:${REDIS_PASSWORD_URLENC:-${REDIS_PASSWORD}}@redis:6379 http://a@b'
check secrets-outside-urls accept '      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}' \
  '      - GITHUB_TOKEN=${GITHUB_TOKEN}' '      # - X=postgres://u:${POSTGRES_PASSWORD}@h' \
  '      - HEALTH=http://127.0.0.1:3107/tokens/health' \
  '      - KEYCLOAK_URL=http://${KEYCLOAK_HOST}:8080' \
  '    healthcheck:' '      test:' '        - CMD-SHELL' '        - |' \
  '          wget -q http://127.0.0.1:3107/health' '          redis-cli -a ${REDIS_PASSWORD} ping'

if [ "$fail" -ne 0 ]; then echo "test-lint-compose-password-urls: FAILED" >&2; exit 1; fi
echo "test-lint-compose-password-urls: OK"
