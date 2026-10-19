// Package pki is the coordinator PKI of the CI fabric (P7-NODE-07, Epic D5, ADR 0024 §3, ADR 0025 §10).
//
// One root CA (ECDSA P-256, 10 years) signs three intermediates: node, user and coordinator. Leaves carry
// a URI SAN nself-ci:<kind>/<id>. Node and user leaves are ClientAuth, the coordinator leaf ServerAuth.
// Private keys of the hierarchy and of the job-token signer are sealed with AES-256-GCM under a 32-byte
// key-encryption key in <data>/pki/kek (0600, its own file, never in the store, never in a backup).
//
// # Threat model
//
// LAN man in the middle at enrollment. The enrollment listener serves TLS 1.3 with the coordinator leaf
// and the chain up to the root. The joining node cannot know the root yet, so it shows the root
// fingerprint (SHA-256 of the root DER, 256 bits, OpenSSH form) and needs an interactive "yes" or an
// exact --expect-fingerprint. Only then does it send the code and CSR, over a connection verified
// against that root. A proxy with its own root changes the fingerprint; a proxy with the real root and
// its own leaf fails chain verification. No fingerprint and no terminal is a refusal.
//
// Pairing-code brute force. A code is 50 bits of crypto/rand, stored as a SHA-256 hash, single use,
// valid 10 minutes. Five wrong codes lock every live offer (store.MaxCodeAttempts), and the listener
// admits one request per second per source address. A locked offer is a denial of service by design:
// the operator makes a new one.
//
// CA key theft. Keys are sealed; stealing the database alone yields nothing. Stealing database and KEK
// is a full compromise, so the KEK is excluded from backups (P7-NODE-20) and the guide says so.
//
// Replayed CSR. A CSR proves possession of its key only. Issuance needs the single-use code AND an
// approver decision on the exact CSR key fingerprint, which the node printed. A second redemption of a
// code revokes the certificate issued for the first and raises an alert.
//
// Clock skew. Certificates are backdated one minute; join and the enrollment route refuse when the
// clocks differ by more than 120 s (E660 clock_skew). Short-lived node certificates (7 days) renew at
// 2/3 of life; an expired one cannot renew.
//
// Resumed TLS and live sessions. Every server config disables session tickets and checks kind, the
// exact intermediate, the deny list and node state in VerifyConnection (every handshake). RequestGuard
// re-checks each request against an in-memory deny list that follows the store within one refresh
// interval and cancels the requests of a certificate the moment it is denied.
//
// Not in scope: PAKE (no maintained, reviewed Go CPace library; TOFU with SAS confirmation is the ADR
// 0024 §3 fallback), intermediate renewal (5-year lifetime), CRL/OCSP (the deny list is the revocation
// mechanism and all peers are ours).
package pki
