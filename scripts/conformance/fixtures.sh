#!/usr/bin/env bash
# fixtures.sh (check.sh --fixtures): convert the v1 fixtures with the codemods, require the gate to PASS on
# every converted fixture (all rules, including the vendored build from the extracted tarball), then plant
# each defect on a copy and require the named rule to FAIL.
set -u
. "$(dirname "$0")/lib.sh"
work=$(conf_work)/gate-fixtures; conv=$work/converted; mut=$work/mut; rc=0
rm -rf "$work"; mkdir -p "$work"
bash "$CONF_ROOT/scripts/codemods/run.sh" --fixtures --out "$conv" >"$work/codemods.log" 2>&1 || { cat "$work/codemods.log"; echo "fixtures: codemods failed"; exit 1; }
for p in alpha beta gamma; do
  if out=$(bash "$CONF_DIR/check.sh" --root "$conv" --plugins "$p" 2>&1); then echo "pass  converted fixture $p"
  else echo "FAIL  converted fixture $p should pass:"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
done
edit() { local f=$1; shift; sed -e "$@" "$f" > "$f.tmp" && mv "$f.tmp" "$f"; }
sock() { local f=${1:-docker-compose.plugin.yml}; awk '/^    networks:/ { print "    volumes:"; print "      - /var/run/docker.sock:/var/run/docker.sock" } { print }' "$f" > c.tmp && mv c.tmp "$f"; }
jqe() { jq "$1" plugin.json > p.tmp && mv p.tmp plugin.json; }
# plant <id> <plugin> <rule> <shell snippet run inside the copied plugin dir> [extra check.sh args]
plant() {
  local id=$1 p=$2 rule=$3 snip=$4; shift 4
  rm -rf "$mut/$id"; mkdir -p "$mut/$id"; cp -R "$conv/$p" "$mut/$id/$p"
  ( cd "$mut/$id/$p" && eval "$snip" )
  local out; out=$(bash "$CONF_DIR/check.sh" --root "$mut/$id" --plugins "$p" --rules "$rule" "$@" 2>&1); local r=$?
  if [ $r -ne 0 ] && printf '%s\n' "$out" | grep -q "FAIL $rule "; then echo "pass  defect $id fails rule $rule"
  else echo "FAIL  defect $id: rule $rule did not fail (rc=$r)"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
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
plant healthcheck-none alpha healthcheck "edit Dockerfile 's/^HEALTHCHECK.*/HEALTHCHECK NONE/'; edit docker-compose.plugin.yml '/^    healthcheck:/,/^    networks:/{/^    networks:/!d;}'"
plant healthcheck-disabled alpha healthcheck "awk '{ print } /^    healthcheck:/ { print \"      disable: true\" }' docker-compose.plugin.yml > c.tmp && mv c.tmp docker-compose.plugin.yml"
plant healthcheck-none-test alpha healthcheck "edit docker-compose.plugin.yml 's/^      test: .*/      test: [\"NONE\"]/'"
plant replace-quoted alpha replace "printf '\nreplace example.com/other => \"../outside\"\n' >> go.mod"
plant replace-gowork-use alpha replace "printf 'go 1.25\n\nuse ../outside\n' > go.work"
plant replace-gowork-block alpha replace "printf 'go 1.25\n\nuse (\n\t.\n\t\"../outside\"\n)\n' > go.work"
plant replace-gowork-replace alpha replace "printf 'go 1.25\n\nreplace example.com/o => ../outside\n' > go.work"
plant apply-in-comment alpha migrations "rm cmd/boot_migrations.go; printf '\n// migrate.Apply(ctx, pool, opts) is wired later\n/* migrate.Apply( */\n' >> cmd/main.go"
plant apply-in-test-only alpha migrations "mv cmd/boot_migrations.go cmd/boot_migrations_test.go"
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
out=$(bash "$CONF_DIR/check.sh" --root "$conv" --plugins beta --rules exceptions --exceptions "$work/exc-old.yaml" 2>&1) && { echo "FAIL  expired exception did not fail the gate"; rc=1; } || { printf '%s\n' "$out" | grep -q 'FAIL exceptions' && echo "pass  expired exception fails the gate" || { echo "FAIL  expired exception rc but no message"; rc=1; }; }
out=$(CONF_TODAY=2027-06-01 bash "$CONF_DIR/check.sh" --root "$conv" --plugins beta --rules exceptions 2>&1) && { echo "FAIL  committed exceptions.yaml must expire"; rc=1; } || echo "pass  committed exceptions expire after the cap date"
[ $rc -eq 0 ] && echo "fixtures: gate ok" || echo "fixtures: gate FAIL"
exit $rc
