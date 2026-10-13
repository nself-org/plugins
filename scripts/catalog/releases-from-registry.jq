# releases-from-registry.jq: the one-time extraction of release data from the hand-kept registry.json.
#
# Purpose: P7-PLUG-36 moves checksums and tarball URLs out of registry.json (now a generated projection) into
#   releases.json, the file only the release pipeline writes. This filter reads the last hand-kept registry.json and
#   emits the releases.json document; scripts/plugin-counts.sh --init-releases runs it once.
# Input:   a hand-kept registry.json (plugins keyed by slug). Output: releases.json (schema releases.v1).
# Rules:   an entry with no `checksum` has no published bytes, so it gets no row (the generator leaves it out of the registry
#   until the release pipeline adds one). `binaries` is what a released CLI requires of the archive (v1.4.12
#   cliBinaryNames over the entry's cliCommands, binaryName and pluginType), so the generated registry keeps emitting the
#   same linking keys the hand-kept one carried; the release pipeline replaces it with the tarball listing.
def required($slug):
  (.implementation // {}) as $i
  | if ($i.pluginType // "cli") != "cli" then []
    elif ((.cliCommands // []) | length) > 0 then [.cliCommands[].name | "nself-" + .]
    elif ($i.binaryName // "") != "" then [$i.binaryName]
    elif $i.pluginType == "cli" then ["nself-" + $slug]
    else [] end;
{
  _generated: "Release data written by scripts/verify-published-checksums.sh --write from published bytes (first rows: scripts/plugin-counts.sh --init-releases from the hand-kept registry.json). DO NOT HAND EDIT.",
  schema_version: 1,
  releases: (.plugins | to_entries
    | map(select(.value.checksum != null) | .key as $s | .value as $v
      | {key: $s, value: (
          {version: $v.version, sha256: $v.checksum, release_signature: null}
          + (if $v.releaseTag then {release_tag: $v.releaseTag} else {} end)
          + (if $v.tarballUrl then {tarball_url: $v.tarballUrl} else {} end)
          + (if ($v.tarball // "") != "" and $v.tarball != $v.tarballUrl then {tarball: $v.tarball} else {} end)
          + (if $v.checksums.platforms then {platform_checksums: $v.checksums.platforms} else {} end)
          + (($v | required($s)) as $b | if ($b | length) > 0 then {binaries: $b} else {} end))})
    | from_entries)
}
