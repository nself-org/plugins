#!/usr/bin/env bash
# ports-fixtures.sh: ports.sh against synthetic trees (final gate G10). Two plugins on one port exit 1
# naming both owners, a plugin on a reserved port exits 1 naming the reserved owner, a unique port exits 0.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); PORTS=$HERE/../../scripts/conformance/ports.sh
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT; rc=0
mk() { mkdir -p "$tmp/$1/$2"; printf '%s\n' "$3" > "$tmp/$1/$2/plugin.json"; }
mk dup p1 '{"name":"p1","port":3950}';  mk dup p2 '{"name":"p2","port":3950}'
mk res p3 '{"name":"p3","port":3857}'
mk ok  p4 '{"name":"p4","port":3951}';  mk ok p5 '{"name":"p5","port":0}'; mk ok p6 '{"name":"p6"}'
mk cfg p7 '{"name":"p7","port":3960}';  mk cfg p8 '{"name":"p8","port":3961,"config":{"port":3960}}'
mk env p9 '{"name":"p9","port":3962}';  mk env p10 '{"name":"p10","port":3963,"env":{"optional":{"P10_PORT":"3962"}}}'
mk aligned p11 '{"name":"p11","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
mk compose p12 '{"name":"p12","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
mk docker p13 '{"name":"p13","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
for kind in aligned compose docker; do
  plugin=$(find "$tmp/$kind" -name plugin.json -print)
  dir=${plugin%/plugin.json}
  printf 'services:\n  app:\n    ports:\n      - "127.0.0.1:${HOST_PORT:-4970}:${PORT:-3970}"\n' > "$dir/docker-compose.plugin.yml"
  printf 'FROM scratch\nEXPOSE 3970/tcp 9999/udp\n' > "$dir/Dockerfile"
done
printf 'services:\n  app:\n    ports:\n      - "4970:3971"\n' > "$tmp/compose/p12/docker-compose.plugin.yml"
printf 'FROM scratch\nEXPOSE 3971\n' > "$tmp/docker/p13/Dockerfile"
mk flow p14 '{"name":"p14","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
printf 'services:\n  plugin-p14:\n    ports: ["4970:3971"]\n' > "$tmp/flow/p14/docker-compose.plugin.yml"
mk long p15 '{"name":"p15","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
printf 'services:\n  p15:\n    ports:\n      - target: 3971\n        published: "4970"\n' > "$tmp/long/p15/docker-compose.plugin.yml"
mk unknown p16 '{"name":"p16","service":{"port":3970,"compose":"docker-compose.plugin.yml"}}'
printf 'services:\n  p16:\n    ports:\n      - "4970:${PORT}"\n' > "$tmp/unknown/p16/docker-compose.plugin.yml"
expect() { # name want-rc grep-pattern
  local out r; out=$(bash "$PORTS" --all --root "$tmp/$1" 2>&1); r=$?
  if [ "$r" -eq "$2" ] && { [ -z "$3" ] || printf '%s\n' "$out" | grep -Eq "$3"; }; then echo "pass  ports $1 (rc=$r)"; else echo "FAIL  ports $1: rc=$r want $2 pattern '$3'"; printf '%s\n' "$out"; rc=1; fi
}
expect dup 1 '^collision 3950 p1 p2$'
expect res 1 '^collision 3857 p3 reserved:admin_API$|^collision 3857 reserved:admin_API p3$'
expect ok 0 ''
expect cfg 1 '^collision 3960 p7 p8$'
expect env 1 '^collision 3962 p9 p10$|^collision 3962 p10 p9$'
expect aligned 0 ''
expect compose 1 'mismatch.*p12.*compose.*3970.*3971'
expect docker 1 'mismatch.*p13.*Dockerfile.*3970.*3971'
expect flow 1 'mismatch.*p14.*compose.*3970.*3971'
expect long 1 'mismatch.*p15.*compose.*3970.*3971'
expect unknown 1 'mismatch.*p16.*compose.*no numeric'
[ $rc -eq 0 ] && echo "ports: fixtures ok" || echo "ports: fixtures FAIL"
exit $rc
