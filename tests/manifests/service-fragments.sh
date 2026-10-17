#!/usr/bin/env bash
# service-fragments.sh: a free plugin that declares a port either ships a compose fragment that publishes that
# port or says installable:false (D-0816). Without one of the two the manifest v2 codemod reports "manual"
# (service.kind compose, no fragment) and the plugin can never run in a stack or pass the conformance gate.
# Usage: service-fragments.sh [plugin...]   default: the plugins D-0816 named.
# Exit: 0 pass, 1 a plugin has neither, 2 usage.
set -u
WT=$(cd "$(dirname "$0")/../.." && pwd)
plugins=${*:-email event-bus family-gedcom family-ancestry family-familysearch family-myheritage family-wikitree}
rc=0
for p in $plugins; do
  d=$WT/free/$p
  [ -f "$d/plugin.json" ] || { echo "FAIL $p: no plugin.json"; rc=1; continue; }
  port=$(jq -r '.port // empty' "$d/plugin.json")
  [ -n "$port" ] || { echo "ok   $p: declares no port"; continue; }
  if jq -e '.installable == false' "$d/plugin.json" >/dev/null; then echo "ok   $p: installable:false"; continue; fi
  frag=$d/docker-compose.plugin.yml
  if [ ! -f "$frag" ]; then echo "FAIL $p: declares port $port, is installable and ships no docker-compose.plugin.yml"; rc=1; continue; fi
  if ! grep -qE "^[[:space:]]*-[[:space:]]*\"?127\.0\.0\.1:$port:$port\"?" "$frag"; then
    echo "FAIL $p: docker-compose.plugin.yml does not publish 127.0.0.1:$port:$port"; rc=1; continue; fi
  if ! grep -q "^  $p:" "$frag"; then echo "FAIL $p: docker-compose.plugin.yml has no service named $p"; rc=1; continue; fi
  echo "ok   $p: fragment publishes $port"
done
exit $rc
