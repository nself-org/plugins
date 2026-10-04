#!/usr/bin/env bash
# lib.sh - measurement functions for scripts/preflight/run.sh (P7-PLUG-02).
#
# Sourced, never executed. Read-only: HTTPS GETs of public endpoints, local
# extraction and `docker build` (docker.sh; everything it creates is scoped to
# the run id). Nothing is written outside $PF_WORK. Sequential throughout.
#
# Per-entry pipeline, first failing stage wins, class names the stage:
#   404       route answered other than 302, or a download answered other than 200
#   checksum  sha256 of the served bytes != the registry's checksum
#   extract   not a gzip tarball, unsafe member path, or tar exit != 0
#   manifest  plugin.json missing, not JSON, or name/version != registry entry
#   fragment  service plugin without docker-compose.plugin.yml or Dockerfile
#   build     `docker build` of the extracted tree alone failed (or no Dockerfile)
#   drift     served registry differs from registry.json at main (or slug absent);
#             wins over every other class and names it ("; also <class>: ...")
#
# Constitution 8.4: a skipped stage is a skip, never a pass.
#
# Variables the caller (run.sh) sets: PF_WORK PF_ENTRIES PF_SERVED_URL
# PF_MAIN_TABLE PF_SERVED_TABLE PF_NO_BUILD PF_LOG_DIR (optional).

PF_UA="nself-preflight/1"

pf_log() { printf '[preflight] %s\n' "$*" >&2; }
pf_die() { pf_log "ERROR: $*"; exit 2; }

pf_need() {
  local c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || pf_die "missing required tool: $c"
  done
}

pf_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

pf_sha256_str() { printf '%s' "$1" | shasum -a 256 | cut -d' ' -f1; }

# pf_curl_code OUTFILE URL [curl args...]: prints the HTTP status (000 on a
# transport failure). Retries 000/429/5xx up to 3 times. No `curl -f`: the
# status is the per-item evidence.
pf_curl_code() {
  local out="$1" url="$2" code attempt=0
  shift 2
  while :; do
    code=$(curl -sS --max-time "${PF_CURL_TIMEOUT:-300}" -A "$PF_UA" \
      -o "$out" -w '%{http_code}' "$@" "$url" 2>/dev/null) || code=000
    case "$code" in
      000 | 429 | 5??)
        attempt=$((attempt + 1))
        [ "$attempt" -lt 3 ] || break
        sleep 2
        ;;
      *) break ;;
    esac
  done
  printf '%s' "$code"
}

# pf_registry_table FILE: sorted TSV  slug version installable url checksum
# Accepts the served shape (plugins array) and the repo shape (plugins object).
# "-" stands for an absent value so `read` with a tab IFS cannot collapse it.
pf_registry_table() {
  jq -r '(.plugins | if type == "array" then . else (to_entries | map(.value + {name: (.value.name // .key)})) end)
    | .[]
    | [ .name, (.version // "-"),
        (if .installable == false then "false" else "true" end),
        (.tarball // "-"),
        (.checksum // .checksums.sha256 // "-") ]
    | @tsv' "$1" | LC_ALL=C sort
}

# pf_fetch_cached URL: download once per distinct URL per run. Sets
# PF_DL_FILE and PF_DL_CODE. Follows redirects (GitHub asset CDN); the code is
# the final response's.
pf_fetch_cached() {
  local key
  key=$(pf_sha256_str "$1")
  PF_DL_FILE="$PF_WORK/dl/$key.bin"
  if [ -f "$PF_WORK/dl/$key.code" ]; then
    PF_DL_CODE=$(cat "$PF_WORK/dl/$key.code")
    return 0
  fi
  PF_DL_CODE=$(pf_curl_code "$PF_DL_FILE" "$1" -L)
  [ "$PF_DL_CODE" = 200 ] || rm -f "$PF_DL_FILE"
  printf '%s' "$PF_DL_CODE" >"$PF_WORK/dl/$key.code"
}

# pf_drift SOURCE SLUG VERSION WANT: prints the drift description (empty when
# the entry equals its counterpart in the other registry). Needs only the tables.
pf_drift() {
  local own other row t=$'\t'
  if [ "$1" = served ]; then other="$PF_MAIN_TABLE"; own="the served registry"; else other="$PF_SERVED_TABLE"; own="registry.json at main"; fi
  row=$(awk -F'\t' -v s="$2" '$1 == s {print $2 "\t" $5; exit}' "$other")
  if [ -z "$row" ]; then
    if [ "$1" = served ]; then printf 'drift: served registry lists %s but registry.json at main does not' "$2"
    else printf 'drift: registry.json at main lists %s but the served registry does not' "$2"; fi
    return 0
  fi
  [ "$row" = "$3$t$4" ] || printf 'drift: %s has %s/%s, the other registry has %s' "$own" "$3" \
    "$(printf '%s' "$4" | cut -c1-12)" "$(printf '%s' "$row" | awk -F'\t' '{print $1 "/" substr($2,1,12)}')"
}

# pf_done RESULT CLASS REASON: append one entry. Reads source, slug, version,
# route, dl, sha and drift from the calling pf_check_entry frame (bash dynamic
# scope). A drifting entry is never a pass and always reports class drift, with
# any other failure kept in the reason, so P7-PLUG-35 routing cannot be masked.
pf_done() {
  local res="$1" cls="$2" why="$3"
  if [ -n "$drift" ]; then
    if [ "$res" = pass ]; then why="$drift"; else why="$drift; also ${cls:-none}: $3"; fi
    res=fail cls=drift
  fi
  jq -nc --arg source "$source" --arg slug "$slug" --arg version "$version" \
    --arg result "$res" --arg class "$cls" --arg reason "$why" \
    --arg route "$route" --arg dl "$dl" --arg sha "$sha" \
    '{source: $source, slug: $slug, version: $version, result: $result,
      class: (if $class == "" then null else $class end), reason: $reason,
      http: {route: (if $route == "" then null else ($route | tonumber) end),
             download: (if $dl == "" then null else ($dl | tonumber) end)},
      sha256: (if $sha == "" then null else $sha end)}' >>"$PF_ENTRIES"
}

# pf_check_entry SOURCE SLUG VERSION INSTALLABLE URL WANT_SHA
# SOURCE is "served" (follow GET <served>/plugins/<slug>/tarball) or "main"
# (the entry's own tarball URL from registry.json at main).
pf_check_entry() {
  local source="$1" slug="$2" version="$3" installable="$4" url="$5" want="$6"
  local route="" dl="" sha="" dir="$PF_WORK/x" manifest root kind mname mver missing rc drift
  drift=$(pf_drift "$source" "$slug" "$version" "$want")
  if [ "$installable" = false ]; then
    if [ "$url" = "-" ]; then
      pf_done pass "" "declared installable:false and carries no tarball: nothing to download by design"
    else
      pf_done fail manifest "declared installable:false but the registry entry carries a tarball URL"
    fi
    return 0
  fi
  if [ "$source" = served ]; then
    route=$(pf_curl_code /dev/null "$PF_SERVED_URL/plugins/$slug/tarball" -D "$PF_WORK/hdr")
    if [ "$route" != 302 ]; then
      pf_done fail 404 "tarball route answered HTTP $route (expected 302)"
      return 0
    fi
    url=$(tr -d '\r' <"$PF_WORK/hdr" | awk 'tolower($1) == "location:" {print $2; exit}')
  fi
  if [ -z "$url" ] || [ "$url" = "-" ]; then
    pf_done fail 404 "no tarball URL to download (registry entry or route Location is empty)"
    return 0
  fi
  pf_fetch_cached "$url"
  dl="$PF_DL_CODE"
  if [ "$dl" != 200 ]; then
    pf_done fail 404 "tarball download answered HTTP $dl"
    return 0
  fi
  sha=$(pf_sha256 "$PF_DL_FILE")
  if [ "$want" = "-" ]; then
    pf_done fail checksum "registry entry carries no checksum (served sha256 $(printf '%s' "$sha" | cut -c1-12))"
    return 0
  fi
  if [ "$sha" != "$want" ]; then
    pf_done fail checksum "served sha256 $(printf '%s' "$sha" | cut -c1-12) != registry $(printf '%s' "$want" | cut -c1-12)"
    return 0
  fi

  rm -rf "$dir"
  mkdir -p "$dir"
  if ! tar -tzf "$PF_DL_FILE" >"$PF_WORK/list" 2>/dev/null; then
    pf_done fail extract "not a readable gzip tarball (tar -t failed)"
    return 0
  fi
  if grep -Eq '^/|(^|/)\.\.(/|$)' "$PF_WORK/list"; then
    pf_done fail extract "tarball has an absolute or parent-relative member path"
    return 0
  fi
  rc=0
  tar -xzf "$PF_DL_FILE" -C "$dir" >/dev/null 2>&1 || rc=$?
  if [ "$rc" != 0 ]; then
    pf_done fail extract "tar -x exit $rc"
    return 0
  fi

  manifest=$(find "$dir" -maxdepth 3 -name plugin.json -type f | awk '{print length($0) " " $0}' | sort -n | head -1 | cut -d' ' -f2-)
  if [ -z "$manifest" ]; then
    pf_done fail manifest "no plugin.json in the tarball"
    return 0
  fi
  root=$(dirname "$manifest")
  if ! jq -e 'type == "object"' "$manifest" >/dev/null 2>&1; then
    pf_done fail manifest "plugin.json is not a valid JSON object"
    return 0
  fi
  mname=$(jq -r '.name // ""' "$manifest")
  mver=$(jq -r '.version // ""' "$manifest")
  if [ "$mname" != "$slug" ] || [ "$mver" != "$version" ]; then
    pf_done fail manifest "plugin.json says $mname@$mver, registry entry is $slug@$version"
    return 0
  fi

  kind=$(jq -r 'if (.pluginType == "cli") or (.binaryName and (.port | not)) then "cli" else "service" end' "$manifest")
  missing=""
  if [ "$kind" = service ]; then
    [ -f "$root/docker-compose.plugin.yml" ] || missing="docker-compose.plugin.yml"
    [ -f "$root/Dockerfile" ] || missing="${missing:+$missing, }Dockerfile"
    if [ -n "$missing" ]; then
      pf_done fail fragment "service plugin tarball lacks: $missing"
      return 0
    fi
  elif [ ! -f "$root/Dockerfile" ]; then
    pf_done fail build "cli plugin tarball has no Dockerfile: nothing to build from the tarball alone"
    return 0
  fi

  pf_build "$root" "$sha" "$slug"
  if [ "$PF_B_RESULT" != pass ]; then
    pf_done "$PF_B_RESULT" "$PF_B_CLASS" "$PF_B_REASON"
    return 0
  fi

  pf_done pass "" "tarball downloaded (HTTP 200), sha256 equals the registry, extracted, manifest and fragment ok, docker build ok, no registry drift"
}
