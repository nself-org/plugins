# DEPRECATION-POLICY — Free Plugins

Governs end-of-life (EOL) notice periods and procedures for plugins in
`plugins/` (MIT-licensed, free tier). (S58-T07)

For paid plugin policy see `plugins-pro/DEPRECATION-POLICY.md`.
Full doctrine: `.claude/docs/doctrines/plugin-lifecycle.md`.

---

## Minimum Notice Periods

| Tier | Min notice | Extra requirements |
|------|------------|--------------------|
| **Free (MIT)** | 6 months from `announcedDate` to `eolDate` | deprecation block in plugin.json |
| **Security-critical removal** | 0-day allowed | announcement must state "security-critical removal" |

## Transition Process

1. Open PR setting `status: deprecated` in `plugin.json` and `registry.json`.
2. Include complete `deprecation` block: `announcedDate`, `eolDate` (min 6 months
   from announcedDate), `migrationGuide` (URL that resolves HTTP 200), and
   optionally `replacedBy` + `migrationScript`.
3. CI gate validates the block and checks URL resolution — PR is blocked otherwise.
4. After `eolDate`, status may be changed to `eol`. The plugin is never physically
   deleted from the registry during the 24-month archive window; old tarballs are
   preserved at `releases/archive/<plugin>-<version>.tgz`.

## Announcement Channels

- GitHub Release notes on the `plugins` repo.
- CHANGELOG entry with `[DEPRECATED]` tag.
- Migration guide published at the URL in `migrationGuide` before the PR lands.

## Security Exception

An active, exploited vulnerability with no available patch allows immediate
removal (0-day). Removal announcement must:
- Explicitly state "security-critical removal".
- List the CVE or equivalent vulnerability identifier.
- Provide a mitigation workaround in the announcement body.

Non-critical disclosures still require the standard 6-month notice period.

---

## Plugin Rename Policy

A plugin rename occurs when an existing plugin is replaced by a new canonical name as part of
a consolidation or naming reconciliation. Unlike an EOL deprecation, a rename does not remove
functionality; it moves it to a new plugin identifier.

### Scope

This section applies to any plugin whose name changes between nSelf releases. It does NOT
apply to plugins being discontinued without replacement (those follow the standard EOL process
above).

### Retired Aliases — P4 Rename Batch

The P4 Gateway Unification (E1) consolidated five overlapping plugin names into three canonical
plugins. The following aliases are retired as of P4:

| Retired plugin name | Canonical replacement | Canonical port | Action for operators |
|---|---|---|---|
| `plugin-ai` | `nself-ai-gateway` | 3761 | Update `plugin.yaml` installs and any `PLUGIN_AI_*` env var references; see migration guide below |
| `plugin-pty` | `nself-ai-cc` | 3760 | Replace `nself plugin install plugin-pty` with `nself plugin install nself-ai-cc` |
| `plugin-llm-gateway` | `nself-ai-gateway` | 3761 | Same as `plugin-ai` above |
| `plugin-clawde` (gateway stubs only) | `nself-ai-cc` | 3760 | Only the gateway stub file is retired; PTY relay service remains (see E3 for full service) |
| `plugin-retrieval` (gateway alias only) | `nself-ai-gateway` | 3761 | Only the gateway routing alias is retired; pure retrieval service moves to E3 |

### Migration Path

1. **Uninstall the retired alias:**
   ```bash
   nself plugin remove plugin-ai
   # or: nself plugin remove plugin-pty
   # or: nself plugin remove plugin-llm-gateway
   ```

2. **Install the canonical replacement:**
   ```bash
   nself plugin install nself-ai-gateway   # replaces plugin-ai and plugin-llm-gateway
   nself plugin install nself-ai-cc        # replaces plugin-pty
   nself plugin install nself-ai-mcp       # new; no predecessor for free-tier users
   ```

3. **Update env var references:**
   Old `PLUGIN_AI_*` env vars are no longer read. See
   `plugins-pro/.github/docs/nself-ai-gateway.md` § Environment Variables for the canonical
   variable names.

4. **After P4 ships:**
   Retired plugin names return `404` from `plugins.nself.org` and will not install.
   The 24-month archive window does NOT apply to renames: the source code moves to the
   canonical plugin, and the old tarball is not preserved.

### Future Renames

Any future plugin rename must:
- File a PCI of type `breaking-change` against the relevant repo before the P-plan is finalized.
- Publish a migration guide at a resolving URL before the rename PR lands.
- Add a row to this table in the same PR.
- Update `SPORT F03` (alias map) and `SPORT F04` (plugin inventory) in the same commit as the FEATURES.md change (per the E1 Commit Sequencing Hard Rule).
