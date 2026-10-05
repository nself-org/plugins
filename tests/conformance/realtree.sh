#!/usr/bin/env bash
# realtree.sh: idempotence of the codemods on the REAL free/ tree (P7-PLUG-03).
# Usage: realtree.sh [--drop-dev-tool-keys] [--plugins a,b | --all] [--work DIR]
# Copies the checkout (without .git) to a scratch git repo, runs run.sh twice and requires: run 2 reports no
# "converted", run 2 changes zero files, a plugin left unconverted (blocked, manual, skipped) is byte-identical
# to the original after run 1, and each run covered the plugins asked for (a run over nothing fails).
set -u
HERE=$(cd "$(dirname "$0")" && pwd); WT=$(cd "$HERE/../.." && pwd)
flag=; sel=--all; work=${TMPDIR:-/tmp}/p3-realtree; rc=0
while [ $# -gt 0 ]; do case $1 in
  --drop-dev-tool-keys) flag=--drop-dev-tool-keys;; --plugins) sel="--plugins $2"; shift;; --all) sel=--all;;
  --work) work=$2; shift;; *) echo "realtree.sh: unknown argument $1" >&2; exit 2;; esac; shift; done
T=$work/tree; export CONF_WORKDIR=$work/conf
rm -rf "$work"; mkdir -p "$T" "$CONF_WORKDIR"
( cd "$WT" && tar cf - --exclude=.git --exclude=node_modules --exclude=.claude . ) | ( cd "$T" && tar xf - )
cd "$T" || exit 9
git init -q . && git add -A >/dev/null 2>&1 && git -c user.email=t@t -c user.name=t commit -qm base || { echo "FAIL: baseline commit"; exit 9; }
tree_hash() { find free -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256 | cut -d' ' -f1; }
want=$(case "$sel" in --all) ls -d free/*/ | wc -l | tr -d ' ';; *) printf '%s' "${sel#--plugins }" | tr ',' '\n' | grep -c .;; esac)
bash scripts/codemods/run.sh $flag $sel > run1.txt 2>&1; echo "run1 rc=$?"
n1=$(grep -c '^plugin ' run1.txt); echo "run1 plugins=$n1 (asked for $want)"
{ [ "$n1" -gt 0 ] && [ "$n1" -ge "$want" ]; } || { echo "FAIL: run 1 covered $n1 of $want plugins"; rc=1; }
grep '^summary' run1.txt | cut -c1-200
h1=$(tree_hash); bad=0
for p in $(grep -E '^plugin .* (manifest=(blocked|manual|skipped)|vendor=(blocked|manual)) ' run1.txt | awk '{ print $2 }'); do
  st=$(git status --porcelain --ignored -- "free/$p")
  [ -z "$st" ] || { echo "FAIL: $p was left unconverted but modified:"; printf '%s\n' "$st" | head -5; bad=1; }
done
[ $bad -eq 0 ] || rc=1
bash scripts/codemods/run.sh $flag $sel > run2.txt 2>&1; echo "run2 rc=$?"
n2=$(grep -c '^plugin ' run2.txt); [ "$n2" -eq "$n1" ] || { echo "FAIL: run 2 covered $n2 plugins, run 1 $n1"; rc=1; }
conv=$(grep -cE '^plugin .*=converted' run2.txt); echo "run2 plugin lines with converted: $conv"
[ "$conv" -eq 0 ] || { echo "FAIL: second run converted again:"; grep -E '^plugin .*=converted' run2.txt | head; rc=1; }
h2=$(tree_hash); [ "$h1" = "$h2" ] && echo "run2 changed zero files (tree hash equal)" || { echo "FAIL: run 2 changed files"; rc=1; }
norm() { grep '^plugin ' "$1" | sed 's/converted/X/g;s/noop/X/g'; }
if diff <(norm run1.txt) <(norm run2.txt) >/dev/null; then echo "run2 states match run1 (converted and noop aside)"; else echo "FAIL: run 2 reports different states than run 1:"; diff <(norm run1.txt) <(norm run2.txt) | head -10; rc=1; fi
[ $rc -eq 0 ] && echo "realtree: ok" || echo "realtree: FAIL"; exit $rc
