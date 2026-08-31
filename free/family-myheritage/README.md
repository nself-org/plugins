# family-myheritage (PLANNED)

MyHeritage → nFamily migration helper. Status: `planned` — scaffold only, no implementation.

MyHeritage is the parent company of Geni.com — same ownership, similar data model, cleanest legal path for migration. API: https://faq.myheritage.com/en/article/myheritage-api.

**Strategic note:** A nSelf ↔ MyHeritage partnership would let nSelf self-hosters inherit bulk-export permission without each operator needing to email for approval. Business-dev question, not engineering. Captured as open question in `family-geni` scaffold PCI.

Plugin follows the `family-*` helper pattern (core + helper-importer). Writes to `np_family_*` tables in the `family` plugin.

Port: 3511. Category: social. Tier: pro.
