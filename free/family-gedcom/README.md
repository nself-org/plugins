# family-gedcom (PLANNED, FREE)

Generic GEDCOM file importer for the `family` plugin. Status: `planned` — scaffold only, no implementation. MIT-licensed free plugin.

Accepts any GEDCOM 5.5.1 or 7.0 file from any provider (Geni, Ancestry, FamilySearch, MyHeritage, WikiTree, Family Tree Builder, etc.) plus an optional photo-folder upload. Matches OBJE pointers in the GEDCOM to files in the folder.

**Why free:**
- Showcases the core (`family`, paid) + helper-importer (free GEDCOM companion) pattern
- Gives non-paying users a read-only migration path (bring your own GEDCOM → get a family tree view)
- Reduces friction to adopt the paid `family` plugin
- No provider lock-in — works with any GEDCOM emitter

Plugin follows the `family-*` helper pattern. Writes to `np_family_*` tables owned by the `family` plugin.

Port: 3108. Category: social. Tier: free (MIT). (Port 3108 picked because it was originally proposed for `family` but moved to 3504 in the social cluster. 3108 was the Downloads-era proposal before the social-cluster consolidation on 2026-04-18.)

**Next step:** Implementation sprint is a candidate for post-P93.
