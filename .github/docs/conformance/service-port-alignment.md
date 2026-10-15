# Service port alignment

`scripts/conformance/ports.sh` checks port uniqueness and compares numeric
`service.port` values with the primary compose service's container targets and
the final Dockerfile stage's `EXPOSE` declarations. Host published ports may differ.
Numeric environment defaults in compose mappings are supported. A declared ports
block without a numeric target fails the check. Plugins without a compose fragment
remain supported.

D-0885 aligns the standalone defaults, Dockerfile health checks and compose
fragments with the existing assigned manifest ports: geocoding 3856,
github-runner 3054, idme 3820, link-preview 3736 and hipaa 3214. Existing
environment overrides remain available. This avoids assigning geocoding the
3203 port used by nself-geo or github-runner the 3053 port used by push.

The released v1.4.12 CLI oracle produces identical generated compose, fragment
path lists, internal URLs, seed SQL and installed-plugin listings for this change.
The fragment paths remain the same, so no oracle output delta is needed.

Run the synthetic regression cases with
`bash scripts/conformance/ports.sh --fixtures`. Check the affected plugins with
`bash scripts/conformance/ports.sh --plugins geocoding,github-runner,idme,link-preview,hipaa`.
