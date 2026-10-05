#!/usr/bin/env bash
# inner.sh <variant> <plugins...>: runs INSIDE the golang container (network disabled) for v14-oracle.sh.
# Starts the local registry, installs the plugins with the released v1.4.12 binary, builds the project and
# collects the outputs the oracle compares into /w/out-<variant>/. Env: REMOVE=<plugin to try removing>.
set -u
V=$1; shift
# Runs as the HOST uid (docker run --user), so everything written under /w is readable by the host user on a
# native Linux bind mount; HOME, the Go caches and the project live under /tmp, which any uid can write.
export HOME=/tmp/home GOCACHE=/tmp/gocache GOPATH=/tmp/gopath PATH=/w/bin:$PATH NSELF_PLUGIN_REGISTRY=http://127.0.0.1:8099 NSELF_SKIP_SBOM_CHECK=1
mkdir -p /tmp/home /tmp/gocache /tmp/gopath /tmp/rs && cp /w/regsrv.go /tmp/rs/main.go && cd /tmp/rs && { [ -f go.mod ] || go mod init regsrv >/dev/null 2>&1; } && go build -o /tmp/regsrv . || exit 5
( /tmp/regsrv "/w/reg-$V" >/dev/null 2>&1 & )
sleep 1
mkdir -p /tmp/proj && cd /tmp/proj && nself init --non-interactive --name oracle --quiet </dev/null >/dev/null 2>&1 || { echo "INIT-FAILED"; exit 2; }
for n in "$@"; do
  nself plugin install "$n" </dev/null >"/tmp/install-$n.log" 2>&1 || { echo "INSTALL-FAILED $n"; tail -8 "/tmp/install-$n.log"; exit 3; }
done
nself build </dev/null >/tmp/build.log 2>&1 || { echo "BUILD-FAILED"; tail -8 /tmp/build.log; exit 4; }
o=/w/out-$V; rm -rf "$o"; mkdir -p "$o"
cp docker-compose.yml "$o/docker-compose.yml"; cp .nself/compose-files.txt "$o/compose-files.txt"
grep '^PLUGIN_.*_INTERNAL_URL=' .nself/compose.env | sort > "$o/compose-env-urls.txt"
cp postgres/init/05-np-plugins-seed.sql "$o/seed.sql"
nself plugin list --installed --detailed </dev/null > "$o/list.txt" 2>&1
if [ -n "${REMOVE:-}" ]; then nself plugin remove "$REMOVE" </dev/null > "$o/remove-guard.txt" 2>&1; echo "rc=$?" >> "$o/remove-guard.txt"; fi
chmod -R a+rX "$o" 2>/dev/null
echo "OK $V"
