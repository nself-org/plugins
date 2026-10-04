#!/usr/bin/env bash
# report.sh - render the markdown summary of a preflight JSON report.
#
# Sourced by run.sh. Pure function of the JSON report and owners.tsv, so the
# markdown is as deterministic as the JSON (the run timestamp is in the header).
#
# Follow-up rule for every non-pass entry (acceptance: each failure names one):
#   served drift (class drift, or the served route itself answering != 302)
#                       -> P7-PLUG-35 (the worker/registry deploy Ticket)
#   anything else       -> the batch Ticket that owns the plugin (owners.tsv)
#   unowned slug        -> "new debt id" (file one in hq quality/debt.yaml)

# pf_render_md REPORT_JSON OWNERS_TSV
pf_render_md() {
  jq -r --rawfile owners "$2" '
    ($owners | split("\n") | map(select(length > 0 and (startswith("#") | not)) | split("\t") | {(.[0]): .[1]}) | add // {}) as $own
    | def follow:
        if (.source == "served") and (.class == "drift" or (.class == "404" and .http.route != 302))
        then "P7-PLUG-35 (served drift)"
        else ($own[.slug] // "new debt id (slug is outside batches P7-PLUG-40..47)") end;
      def cell: tostring | gsub("\\|"; "/");
      . as $r
    | "# P7-PLUG-02 served-bytes preflight (free tier)\n",
      "- Run at: \($r.header.run_at)",
      "- Served registry: \($r.header.served_url)/registry.json?tier=free",
      "- Main registry: registry.json at \($r.header.main_ref) (\($r.header.main_sha))",
      "- Result: **\(if $r.summary.pass then "PASS" else "FAIL" end)** (\($r.summary.counts.pass) pass, \($r.summary.counts.fail) fail, \($r.summary.counts.skip) skip of \($r.summary.expected) expected entries)",
      "- A skip counts as a failure (Constitution 8.4); `installable:false` entries are asserted, not skipped.\n",
      "## Coverage\n",
      "| source | expected | measured | pass | fail | skip |",
      "|---|---|---|---|---|---|",
      (["served", "main"][] as $s
        | ($r.entries | map(select(.source == $s))) as $e
        | "| \($s) | \($r.summary["expected_" + $s]) | \($e | length) | \($e | map(select(.result == "pass")) | length) | \($e | map(select(.result == "fail")) | length) | \($e | map(select(.result == "skip")) | length) |"),
      "\n## Non-pass entries by class\n",
      "| class | served | main |",
      "|---|---|---|",
      (($r.entries | map(select(.result != "pass") | .class) | unique)[] as $c
        | "| \($c) | \($r.entries | map(select(.result != "pass" and .class == $c and .source == "served")) | length) | \($r.entries | map(select(.result != "pass" and .class == $c and .source == "main")) | length) |"),
      "\n## Non-pass entries\n",
      "| source | slug | version | result | class | reason | follow-up |",
      "|---|---|---|---|---|---|---|",
      ($r.entries[] | select(.result != "pass")
        | "| \(.source) | \(.slug) | \(.version) | \(.result) | \(.class) | \(.reason | cell) | \(follow) |")
  ' "$1"
}
