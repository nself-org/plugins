#!/usr/bin/env bash
# migration-lib.sh: helpers shared by the migration-immutable, migration-idempotent and legacy-renames rules (D-0346).
# Source it after lib.sh; it defines functions only. The "applied set" is what the vendored sdk/go/v2/migrate applies at
# boot: top-level migrations/*.sql without the *.down.sql files, in byte order. Bash 3.2 compatible.

# applied_files <plugin dir>: basenames of the applied set, one per line, in byte order.
applied_files() {
  [ -d "$1/migrations" ] || return 0
  find "$1/migrations" -maxdepth 1 -type f -name '*.sql' ! -name '*.down.sql' | sed 's|.*/||' | LC_ALL=C sort
}

# released_boot <baseline plugin dir>: 0 when that plugin at the last release already applied migrations at boot
# (manifest_version 2 and migrations.apply boot), so a boot ledger on a live install may hold its files; 1 when it did not
# (no manifest, or one that says otherwise); 2 when the manifest exists but cannot be read (D-0619 F8): the caller must
# fail the gate, since "not released" would switch the frozen-file check off.
released_boot() {
  [ -f "$1/plugin.json" ] || return 1
  local v; v=$(jq -r 'if .manifest_version == 2 then (.migrations.apply // "") else "" end' "$1/plugin.json" 2>/dev/null) || return 2
  [ "$v" = boot ]
}
# short_sha <file>: first 12 hex digits of the file's sha256.
short_sha() { sha256_of "$1" | cut -c1-12; }
