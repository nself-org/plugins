#!/usr/bin/env bash
# build-from-tarball.sh <plugin dir>: deterministic-tar the plugin, extract it into an empty directory and
# `docker build` there with the network disabled for RUN steps, so a build that needs anything not in the
# tarball (a replace target outside the dir, an unvendored module) fails here. Sequential; cleans its image and,
# when CI=true (a CI runner, never a developer machine), prunes the BuildKit cache so a long shard cannot fill the disk (D-0973).
set -u
. "$(dirname "$0")/lib.sh"
dir=$(cd "$1" && pwd); name=$(plugin_name "$dir")
if [ ! -f "$dir/Dockerfile" ]; then
  if [ -n "$(jq -r '.service.image // empty' "$dir/plugin.json")" ] || [ "$(jq -r '.service.kind // "compose"' "$dir/plugin.json")" != compose ]; then
    echo "ok build $name: no Dockerfile (image-only or non-compose service)"; exit 0
  fi
  fail build "$name" "compose plugin has no Dockerfile and no service.image"; exit 1
fi
command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 || { fail build "$name" "docker is not available (a skipped build is a failure)"; exit 1; }
work=$(conf_work)/tarball-$$; rm -rf "$work"; mkdir -p "$work/x"; tag=nself-conf-$name:$$
cleanup() {
  rm -rf "$work"; docker rmi -f "$tag" >/dev/null 2>&1
  [ "${CI:-}" = true ] && docker builder prune -f >/dev/null 2>&1
  return 0
}
trap cleanup EXIT
bash "$CONF_ROOT/scripts/deterministic-tar.sh" "$work/p.tar.gz" -C "$(dirname "$dir")" "$(basename "$dir")" >/dev/null 2>&1 || { fail build "$name" "deterministic-tar failed"; exit 1; }
tar -xzf "$work/p.tar.gz" -C "$work/x"
# P7-PLUG-44 ruling: tests/boot/build-network (empty marker, same file the boot kit honours) lifts the network ban for a
# plugin whose build must fetch (the Rust plugin: cargo and apk); every Go plugin keeps --network none.
netflag="--network none"; [ -f "$work/x/$(basename "$dir")/tests/boot/build-network" ] && netflag=
if ! out=$(docker build $netflag -q -t "$tag" -f "$work/x/$(basename "$dir")/Dockerfile" "$work/x/$(basename "$dir")" 2>&1); then
  printf '%s\n' "$out" | tail -15 | while IFS= read -r l; do fail build "$name" "$l"; done; exit 1
fi
echo "ok build $name"
