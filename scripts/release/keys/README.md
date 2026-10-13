# Committed release public keys

Public keys only (PEM SPKI Ed25519), named `plugins-release-free-<key id>.pub` where the id is `plugins-<16 hex>` as
derived by `scripts/release/sign.sh`. There is no private key here and never will be.

This directory holds the production free-tier public key `plugins-release-free-2ebca8dfad98e19e.pub`, committed by P7-PLUG-69 after
the owner created the key and the `release-signing` Environment (P7-PROD-59). Rehearsal keys are throwaway and are never committed:
`scripts/release/rehearse.sh` writes its public half next to the tarballs it builds.

The third verification line of P7-PLUG-21 checks this directory: a rehearsal signature must fail
`scripts/release/verify-signatures.sh` against every key committed here.
