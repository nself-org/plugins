# Committed release public keys

Public keys only (PEM SPKI Ed25519), named `plugins-release-free-<key id>.pub` where the id is `plugins-<16 hex>` as
derived by `scripts/release/sign.sh`. There is no private key here and never will be.

This directory holds no key until the first real signed release. The production public key is committed by P7-PLUG-69, after the
owner creates the key and the `release-signing` Environment (P7-PROD-59). Rehearsal keys are throwaway and are never committed:
`scripts/release/rehearse.sh` writes its public half next to the tarballs it builds.

The CI gate for this directory is the third verification line of P7-PLUG-21: a rehearsal signature must fail
`scripts/release/verify-signatures.sh` against every key committed here.
