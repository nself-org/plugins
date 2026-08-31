# family-familysearch (PLANNED)

FamilySearch → nFamily migration helper. Status: `planned` — scaffold only, no implementation.

**Lowest-friction next-step importer after family-geni.** FamilySearch exposes a free public API (https://www.familysearch.org/developers/) with OAuth 2.0 flow. No bulk-pull TOS restrictions as strict as Geni. Large public tree (shared by all users — unlike Geni where each user owns their branch).

Plugin follows the `family-*` helper pattern (core + helper-importer). Writes to `np_family_*` tables in the `family` plugin. Recommended as the second importer to implement.

Port: 3510. Category: social. Tier: pro.

**Next step:** Implementation sprint can proceed once `family` + `family-geni` are shipped and stable.
