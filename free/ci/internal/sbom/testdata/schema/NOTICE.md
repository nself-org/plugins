# Vendored schemas

`bom-1.6.schema.json`, `spdx.schema.json` and `jsf-0.82.schema.json` are the
CycloneDX 1.6 JSON schema and the two schemas it references, copied unchanged
from https://github.com/CycloneDX/specification (`schema/`). The CycloneDX
specification is published under the Apache License 2.0
(https://www.apache.org/licenses/LICENSE-2.0); the files keep their own notices.

They are test inputs only: the run-time check in `validate.go` is structural and
never reads them. Refresh them together when the pinned CycloneDX spec version
changes.
