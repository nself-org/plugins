#!/usr/bin/env bash
# build-from-tarball.sh <plugin dir>: deterministic-tar the plugin, extract it into an empty directory and
# `docker build` there with the network disabled for RUN steps, so a build that needs anything not in the
# tarball (a replace target outside the dir, an unvendored module) fails here. Sequential; cleans its image.
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
trap 'rm -rf "$work"; docker rmi -f "$tag" >/dev/null 2>&1' EXIT
bash "$CONF_ROOT/scripts/deterministic-tar.sh" "$work/p.tar.gz" -C "$(dirname "$dir")" "$(basename "$dir")" >/dev/null 2>&1 || { fail build "$name" "deterministic-tar failed"; exit 1; }
tar -xzf "$work/p.tar.gz" -C "$work/x"
if ! out=$(docker build --network none -q -t "$tag" -f "$work/x/$(basename "$dir")/Dockerfile" "$work/x/$(basename "$dir")" 2>&1); then
  printf '%s\n' "$out" | tail -15 | while IFS= read -r l; do fail build "$name" "$l"; done; exit 1
fi
echo "ok build $name"
