# family-ancestry (PLANNED)

Ancestry.com → nFamily migration helper. Status: `planned` — scaffold only, no implementation.

Blocked on Ancestry.com API access. Ancestry does not expose a public REST API for third-party tree extraction. Requires either:

1. Partnership / official API access with Ancestry (business-dev)
2. Official GEDCOM download by the user + photo-folder upload (fallback mirrors family-geni's GEDCOM path)
3. Browser-automation approach (TOS risk, not recommended)

Plugin follows the `family-*` helper pattern (core + helper-importer). Writes to `np_family_*` tables in the `family` plugin. See `.claude/memory/bundle-architecture.md`.

Port: 3513. Category: social. Tier: pro.

**Next step:** Implementation sprint is gated on user decision about Ancestry API access path.
