#!/usr/bin/env bash
# fixtures.sh (check.sh --fixtures): convert the v1 fixtures with the codemods, require the gate to PASS on
# every converted fixture (all rules, including the vendored build from the extracted tarball), then plant
# each defect on a copy and require the named rule to FAIL. The migration rules (D-0346) compare with a baseline: a copy
# of the converted fixtures (alpha is then a plugin whose release already applied migrations at boot).
set -u
. "$(dirname "$0")/lib.sh"
work=$(conf_work)/gate-fixtures; conv=$work/converted; mut=$work/mut; rc=0
mig=$work/migbase; rm -rf "$work"; mkdir -p "$work"
bash "$CONF_ROOT/scripts/codemods/run.sh" --fixtures --out "$conv" >"$work/codemods.log" 2>&1 || { cat "$work/codemods.log"; echo "fixtures: codemods failed"; exit 1; }
cp -R "$conv" "$mig"; mkdir -p "$work/migempty/zzz" "$work/empty"; echo '{"name":"zzz"}' > "$work/migempty/zzz/plugin.json"   # an older release without alpha
for p in alpha beta gamma; do
  if out=$(bash "$CONF_DIR/check.sh" --root "$conv" --plugins "$p" --migration-base "$mig" 2>&1); then echo "pass  converted fixture $p"
  else echo "FAIL  converted fixture $p should pass:"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
done
edit() { local f=$1; shift; sed -e "$@" "$f" > "$f.tmp" && mv "$f.tmp" "$f"; }
sock() { local f=${1:-docker-compose.plugin.yml}; awk '/^    networks:/ { print "    volumes:"; print "      - /var/run/docker.sock:/var/run/docker.sock" } { print }' "$f" > c.tmp && mv c.tmp "$f"; }
jqe() { jq "$1" plugin.json > p.tmp && mv p.tmp plugin.json; }
# setport <line> [file]: replace the published-port line of the compose fragment (a block of lines is allowed).
setport() { local f=${2:-docker-compose.plugin.yml}; awk -v r="$1" '/^      - "127\.0\.0\.1:39[0-9][0-9]:39[0-9][0-9]"$/ { print r; next } { print }' "$f" > c.tmp && mv c.tmp "$f"; }
portflow() { awk '/^    ports:$/ { print "    ports: [\"3902:3902\"]"; getline; next } { print }' docker-compose.plugin.yml > c.tmp && mv c.tmp docker-compose.plugin.yml; }
hostnet() { awk '{ print } /^    restart:/ { print "    network_mode: host" }' docker-compose.plugin.yml > c.tmp && mv c.tmp docker-compose.plugin.yml; }
# plant <id> <plugin> <rule> <shell snippet run inside the copied plugin dir> [extra check.sh args]
plant() {
  local id=$1 p=$2 rule=$3 snip=$4; shift 4
  rm -rf "$mut/$id"; mkdir -p "$mut/$id"; cp -R "$conv/$p" "$mut/$id/$p"
  ( cd "$mut/$id/$p" && eval "$snip" )
  local out; out=$(bash "$CONF_DIR/check.sh" --root "$mut/$id" --plugins "$p" --rules "$rule" --migration-base "$mig" "$@" 2>&1); local r=$?
  if [ $r -ne 0 ] && printf '%s\n' "$out" | grep -q "FAIL $rule "; then echo "pass  defect $id fails rule $rule"
  else echo "FAIL  defect $id: rule $rule did not fail (rc=$r)"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
}
# pass_plant <id> <plugin> <rules> <snippet> [extra check.sh args]: apply the snippet to a copy, require the rules to PASS.
pass_plant() {
  local id=$1 p=$2 rules=$3 snip=$4; shift 4
  rm -rf "$mut/$id"; mkdir -p "$mut/$id"; cp -R "$conv/$p" "$mut/$id/$p"
  ( cd "$mut/$id/$p" && eval "$snip" )
  local out; out=$(bash "$CONF_DIR/check.sh" --root "$mut/$id" --plugins "$p" --rules "$rules" --migration-base "$mig" "$@" 2>&1)
  if [ $? -eq 0 ]; then echo "pass  fixture $id passes rules $rules"; else echo "FAIL  fixture $id should pass rules $rules:"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
}
plant legacy-key alpha compat "jqe '.bundles = [\"x\"]'"
plant bad-enum alpha schema "jqe '.maturity = \"bogus\"'"
plant compat-drift alpha compat "jqe '.tier = \"pro\"'"
plant missing-healthcheck alpha healthcheck "edit Dockerfile '/HEALTHCHECK/d'; edit docker-compose.plugin.yml '/^    healthcheck:/,/^    networks:/{/^    networks:/!d;}'"
plant outside-replace alpha replace "printf '\nreplace example.com/other => ../outside\n' >> go.mod"
plant unapplied-migration alpha migrations "jqe 'del(.migrations)'"
plant unwired-migration alpha migrations "rm cmd/boot_migrations.go"
plant free-names-pro alpha repository "jqe '.repository = \"https://github.com/nself-org/plugins-pro\"'"
plant licensed-names-plugins gamma repository "jqe '.repository = \"https://github.com/nself-org/plugins\"'"
plant docker-sock beta docker-socket "sock"
plant unvendored-build alpha build "rm -rf vendor"
# Evasions the rules must not miss (review PLUG-03): each planted defect must fail the named rule.
plant sock-in-named-fragment beta docker-socket "mkdir deploy; mv docker-compose.plugin.yml deploy/plugin.yml; jqe '.service.compose = \"deploy/plugin.yml\"'; sock deploy/plugin.yml"
plant sock-in-other-yaml beta docker-socket "mkdir extra; printf 'services:\n  x:\n    volumes:\n      - /var/run/docker.sock:/s\n' > extra/override.yaml"
plant sock-var-run beta docker-socket "mkdir extra; printf 'services:\n  x:\n    volumes:\n      - \"/var/run:/host-run\"\n' > extra/run.yml"
# Rule port-bind (D-0453): every published port binds the literal 127.0.0.1.
plant port-all-interfaces beta port-bind "setport '      - \"3902:3902\"'"
plant port-any-ip beta port-bind "setport '      - \"0.0.0.0:3902:3902\"'"
plant port-container-only beta port-bind "setport '      - \"3902\"'"
plant port-env-host-ip beta port-bind "setport '      - \"\${PLUGIN_BIND:-127.0.0.1}:3902:3902\"'"
plant port-ipv6-any beta port-bind "setport '      - \"[::]:3902:3902\"'"
plant port-long-syntax beta port-bind "setport '      - target: 3902\n        published: 3902'"
plant port-flow-seq beta port-bind "portflow"
plant port-in-other-compose beta port-bind "mkdir extra; printf 'services:\n  x:\n    ports:\n      - \"3999:3999\"\n' > extra/docker-compose.override.yml"
plant port-in-named-fragment beta port-bind "mkdir deploy; mv docker-compose.plugin.yml deploy/plugin.yml; jqe '.service.compose = \"deploy/plugin.yml\"'; setport '      - \"3902:3902\"' deploy/plugin.yml"
plant port-host-network beta port-bind "hostnet"
pass_plant port-long-loopback beta port-bind "setport '      - target: 3902\n        published: 3902\n        host_ip: 127.0.0.1'"
pass_plant port-newline-filename beta port-bind "mkdir meta; printf 'a: 1\n' > 'meta/x
y.yaml'"
pass_plant port-k8s-yaml beta port-bind "mkdir k8s; printf 'kind: Deployment\nspec:\n  template:\n    spec:\n      containers:\n        - name: x\n          ports:\n            - containerPort: 3902\n' > k8s/deploy.yaml"
# Rule boot-schema (D-0453): migrations.apply boot needs schema np_<name>, and the Go boot code must agree.
plant boot-no-schema alpha boot-schema "jqe 'del(.schema)'"
plant boot-wrong-schema alpha boot-schema "jqe '.schema = \"np_other\"'"
plant boot-code-schema-mismatch alpha boot-schema "edit cmd/boot_migrations.go 's/Schema: \"np_alpha\"/Schema: \"np_other\"/'"
pass_plant no-migrations-no-schema beta boot-schema "jqe 'del(.schema)'"
plant healthcheck-none alpha healthcheck "edit Dockerfile 's/^HEALTHCHECK.*/HEALTHCHECK NONE/'; edit docker-compose.plugin.yml '/^    healthcheck:/,/^    networks:/{/^    networks:/!d;}'"
plant healthcheck-disabled alpha healthcheck "awk '{ print } /^    healthcheck:/ { print \"      disable: true\" }' docker-compose.plugin.yml > c.tmp && mv c.tmp docker-compose.plugin.yml"
plant healthcheck-none-test alpha healthcheck "edit docker-compose.plugin.yml 's/^      test: .*/      test: [\"NONE\"]/'"
plant replace-quoted alpha replace "printf '\nreplace example.com/other => \"../outside\"\n' >> go.mod"
plant replace-gowork-use alpha replace "printf 'go 1.25\n\nuse ../outside\n' > go.work"
plant replace-gowork-block alpha replace "printf 'go 1.25\n\nuse (\n\t.\n\t\"../outside\"\n)\n' > go.work"
plant replace-gowork-replace alpha replace "printf 'go 1.25\n\nreplace example.com/o => ../outside\n' > go.work"
plant apply-in-comment alpha migrations "rm cmd/boot_migrations.go; printf '\n// migrate.Apply(ctx, pool, opts) is wired later\n/* migrate.Apply( */\n' >> cmd/main.go"
plant apply-in-test-only alpha migrations "mv cmd/boot_migrations.go cmd/boot_migrations_test.go"
# Migration rules (D-0346): frozen released files, replay-safe new files, renames proven by a fixture.
newsql() { mkdir -p migrations; printf '%s\n' "$2" > "migrations/$1"; }
renfx() { mkdir -p tests/boot; printf '%s\n' "$1" > tests/boot/legacy-renames; }
# Snippets are eval'd inside the plugin copy and read these variables.
# shellcheck disable=SC2034  # the three variables below are read by the eval'd snippets
RENAME='ALTER TABLE IF EXISTS public.alpha_items RENAME TO np_alpha_items2;'
DYN='DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('"'a1', 'b1'), ('a2', 'b2'"')) AS v(o, n) LOOP IF to_regclass('"'public.' || quote_ident(t.o)"') IS NOT NULL THEN EXECUTE format('"'ALTER TABLE public.%I RENAME TO %I'"', t.o, t.n); END IF; END LOOP; END $$;'
GOOD='CREATE INDEX IF NOT EXISTS idx_np_alpha_items_name ON np_alpha_items (name);
INSERT INTO np_alpha_items (id, name) VALUES (100, '"'x'"') ON CONFLICT DO NOTHING;
DO $$ BEGIN IF to_regclass('"'public.np_alpha_items'"') IS NOT NULL THEN EXECUTE '"'ANALYZE np_alpha_items'"'; END IF; END $$;'
plant edit-released alpha migration-immutable "echo '-- touched' >> migrations/001_init.sql"
plant delete-released alpha migration-immutable "rm migrations/002_seed.sql"
plant new-plain-index alpha migration-idempotent 'newsql 003_x.sql "CREATE INDEX idx_np_alpha_items_name ON np_alpha_items (name);"'
plant new-plain-insert alpha migration-idempotent 'newsql 003_x.sql "INSERT INTO np_alpha_items (name) VALUES ('"'third'"');"'
plant new-add-constraint alpha migration-idempotent 'newsql 003_x.sql "ALTER TABLE np_alpha_items ADD CONSTRAINT np_alpha_items_name_chk CHECK (length(name) > 0);"'
plant new-unguarded-do-execute alpha migration-idempotent 'newsql 003_x.sql "DO \$\$ BEGIN EXECUTE '"'CREATE TABLE np_alpha_extra (id int)'"'; END \$\$;"'
plant unreleased-alpha alpha migration-idempotent ":" --migration-base "$work/migempty"
plant rename-no-fixture alpha legacy-renames 'newsql 003_rename.sql "$RENAME"'
plant rename-wrong-pair alpha legacy-renames 'newsql 003_rename.sql "$RENAME"; renfx "alpha_items np_alpha_other"'
plant rename-dynamic-mismatch alpha legacy-renames 'newsql 003_rename.sql "$DYN"; renfx "a1 b1"'
plant probe-without-renames alpha legacy-renames "mkdir -p tests/boot; echo 'SELECT 1;' > tests/boot/legacy-probe.sql"
pass_plant new-guarded alpha migration-immutable,migration-idempotent,legacy-renames 'newsql 003_x.sql "$GOOD"'
pass_plant rename-with-fixture alpha migration-immutable,migration-idempotent,legacy-renames 'newsql 003_rename.sql "$RENAME"; renfx "alpha_items np_alpha_items2"'
pass_plant rename-dynamic-with-fixture alpha migration-idempotent,legacy-renames 'newsql 003_rename.sql "$DYN"; renfx "a1 b1
a2 b2"'
printf 'exceptions:\n  - rule: migration-immutable\n    plugin: alpha\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2026-12-31\n' > "$work/exc-imm.yaml"
pass_plant edit-released-with-exception alpha migration-immutable "echo '-- touched' >> migrations/001_init.sql" --exceptions "$work/exc-imm.yaml"
# Round 1 (plugins#168 review). S1: a baseline that is not a directory of plugin trees must fail, not pass silently.
for b in "$work/nonexistent" "$work/empty"; do
  if out=$(bash "$CONF_DIR/check.sh" --root "$mut/edit-released" --plugins alpha --rules migration-immutable --migration-base "$b" 2>&1); then echo "FAIL  --migration-base $b passed"; rc=1
  elif printf '%s\n' "$out" | grep -q 'FAIL check: migration baseline'; then echo "pass  bad --migration-base ${b#$work/} fails the gate"; else echo "FAIL  bad --migration-base failed without the baseline message"; rc=1; fi
done
# S2: the resolved-tag cache is keyed by the tag's commit; a partial directory is never trusted, the filled one is marked complete.
tag=$(git -C "$CONF_ROOT" describe --tags --abbrev=0 --match 'v*' HEAD 2>/dev/null); sha=$([ -z "$tag" ] || git -C "$CONF_ROOT" rev-parse "$tag^{commit}" 2>/dev/null)
if [ -n "$sha" ]; then
  wd=$work/wd; rm -rf "$wd"; mkdir -p "$wd/migbase-$sha/free"
  out=$(CONF_WORKDIR=$wd bash "$CONF_DIR/check.sh" --root "$mut/edit-released" --plugins alpha --rules migration-immutable 2>&1)
  if [ -f "$wd/migbase-$sha/.complete" ] && ls "$wd/migbase-$sha"/free/*/plugin.json >/dev/null 2>&1 && ! ls "$wd" | grep -q '\.tmp\.' && printf '%s\n' "$out" | grep -q "migration baseline: $tag"; then echo "pass  partial baseline cache is rebuilt, keyed by commit"
  else echo "FAIL  partial baseline cache was trusted or left a tmp dir"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
fi
# N4: a fixture field that is a glob stays literal (set -f), so the pair mismatch is reported, not a cwd expansion.
rm -rf "$mut/glob"; mkdir -p "$mut/glob"; cp -R "$conv/alpha" "$mut/glob/alpha"
( cd "$mut/glob/alpha" && newsql 003_rename.sql "$RENAME" && renfx "alpha_items np_alpha_items2
alpha_items *" )
out=$(cd "$CONF_ROOT" && bash "$CONF_DIR/check.sh" --root "$mut/glob" --plugins alpha --rules legacy-renames 2>&1)
if printf '%s\n' "$out" | grep -q 'in tests/boot/legacy-renames but not in the migrations: alpha_items \*'; then echo "pass  glob in legacy-renames stays literal"; else echo "FAIL  glob in legacy-renames was expanded"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
# A gate that checks no plugin fails; a named plugin that does not exist fails.
mkdir -p "$work/empty"
for args in "--plugins nonexistent" "--all --root $work/empty"; do
  if out=$(bash "$CONF_DIR/check.sh" --root "$conv" $args 2>&1); then echo "FAIL  check.sh $args exited 0:"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; else echo "pass  check.sh $args exits non-zero"; fi
done
# The exceptions cap is fixed in the gate: a file that raises its own cap with a comment still fails.
printf '# cap: 2099-12-31\nexceptions:\n  - rule: docker-socket\n    plugin: beta\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2099-12-31\n' > "$work/exc-2099.yaml"
out=$(bash "$CONF_DIR/check.sh" --root "$conv" --plugins beta --rules exceptions --exceptions "$work/exc-2099.yaml" 2>&1) && { echo "FAIL  an exception expiring 2099 with a self-declared cap passed"; rc=1; } || { printf '%s\n' "$out" | grep -q 'after the v1.5.0 cap' && echo "pass  exception expiring 2099 fails despite a self-declared cap" || { echo "FAIL  2099 exception failed for the wrong reason"; rc=1; }; }
# docker.sock with an unexpired exception passes; with an expired one it fails; an expired entry fails the gate.
printf 'exceptions:\n  - rule: docker-socket\n    plugin: beta\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2026-12-31\n' > "$work/exc-ok.yaml"
printf 'exceptions:\n  - rule: docker-socket\n    plugin: beta\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2020-01-01\n' > "$work/exc-old.yaml"
if bash "$CONF_DIR/check.sh" --root "$mut/docker-sock" --plugins beta --rules docker-socket --exceptions "$work/exc-ok.yaml" >/dev/null 2>&1; then echo "pass  docker.sock with an unexpired exception passes"; else echo "FAIL  unexpired exception did not allow docker.sock"; rc=1; fi
plant docker-sock-expired beta docker-socket "sock" --exceptions "$work/exc-old.yaml"
# port-bind with an unexpired exception passes; with an expired one it fails.
printf 'exceptions:\n  - rule: port-bind\n    plugin: beta\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2026-12-31\n' > "$work/exc-pb-ok.yaml"
printf 'exceptions:\n  - rule: port-bind\n    plugin: beta\n    reason: "fixture"\n    owner_ref: P7-TEST\n    expires: 2020-01-01\n' > "$work/exc-pb-old.yaml"
pass_plant port-all-interfaces-excepted beta port-bind "setport '      - \"3902:3902\"'" --exceptions "$work/exc-pb-ok.yaml"
plant port-all-interfaces-expired beta port-bind "setport '      - \"3902:3902\"'" --exceptions "$work/exc-pb-old.yaml"
out=$(bash "$CONF_DIR/check.sh" --root "$conv" --plugins beta --rules exceptions --exceptions "$work/exc-old.yaml" 2>&1) && { echo "FAIL  expired exception did not fail the gate"; rc=1; } || { printf '%s\n' "$out" | grep -q 'FAIL exceptions' && echo "pass  expired exception fails the gate" || { echo "FAIL  expired exception rc but no message"; rc=1; }; }
out=$(CONF_TODAY=2027-06-01 bash "$CONF_DIR/check.sh" --root "$conv" --plugins beta --rules exceptions 2>&1) && { echo "FAIL  committed exceptions.yaml must expire"; rc=1; } || echo "pass  committed exceptions expire after the cap date"
[ $rc -eq 0 ] && echo "fixtures: gate ok" || echo "fixtures: gate FAIL"
exit $rc
