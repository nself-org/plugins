package doctor

// Purpose: the catalog of node doctor checks CI-DN001..CI-DN015 (P7-NODE-16): stable id, name and the remediation
// every result of that check carries. It is the one table the checks, the tests and the generated code table in
// guides/ci/Reference.md (R-DX-16) read, so a check cannot exist without a remediation.
// Inputs:  none.
// Outputs: Catalog (ordered rows) and Info (one row by id).
// Constraints: ids are zero-padded (CI-50 prefix rule) and never reused; a remediation is a command or an action,
//          never a diagnosis.

// The stable ids (Epic NODE D15: doctor check ids CI-DN001…).
const (
	IDReach     = "CI-DN001"
	IDHostKey   = "CI-DN002"
	IDOpenSSH   = "CI-DN003"
	IDCert      = "CI-DN004"
	IDSkew      = "CI-DN005"
	IDVersion   = "CI-DN006"
	IDDrift     = "CI-DN007"
	IDDisk      = "CI-DN008"
	IDExposure  = "CI-DN009"
	IDDeploy    = "CI-DN010"
	IDBreaker   = "CI-DN011"
	IDDiscovery = "CI-DN012"
	IDUpgrade   = "CI-DN013"
	IDGitHub    = "CI-DN014"
	IDSeparate  = "CI-DN015"
)

// SectionName is the registry name and the `--nodes` selector.
const SectionName = "nodes"

// Entry is one catalog row.
type Entry struct {
	ID          string
	Name        string
	Remediation string
}

var catalog = []Entry{
	{IDReach, "node reachable", "Check the node is powered on and on the network, then run: nself ci nodes test <node>"},
	{IDHostKey, "pinned host key present and matching", "Verify the host key out of band, then run: nself ci nodes rotate <node> --host-key SHA256:<fingerprint>"},
	{IDOpenSSH, "local OpenSSH client 8.0 or newer", "Upgrade the OpenSSH client on this machine to 8.0 or newer (ssh -V shows the version)"},
	{IDCert, "node certificate not near expiry", "Renew the certificate: nself ci nodes rotate <node>, or re-pair the node if it no longer connects"},
	{IDSkew, "node clock within 120 s", "Enable time sync on the node (chrony, systemd-timesyncd or the macOS time server) and re-run the check"},
	{IDVersion, "agent and protocol version at or above the minimum", "Upgrade the agent: nself ci nodes upgrade <node>"},
	{IDDrift, "toolchains agree across nodes", "Install the same toolchain version on the differing nodes, or pin it in .nself/ci.yaml and run: nself ci nodes test <node>"},
	{IDDisk, "free disk and no expired leases", "Free disk space on the node; for expired leases restart the agent (it sweeps job roots whose lease is gone at start)"},
	{IDExposure, "no weak isolation on nodes taking untrusted work", "Re-add the node accepting owner work only (nself ci nodes add <node> --accepts owner), or run untrusted work in a VM"},
	{IDDeploy, "no deploy-host overlap, exclusion refresh current", "Take the node out of the deploy inventory (or revoke it), then run: nself ci nodes test <node>"},
	{IDBreaker, "node not held in maintenance by the breaker", "Fix the cause shown, then run: nself ci nodes enable <node>"},
	{IDDiscovery, "LAN discovery setting", "Set ci.discovery to off in the user config.yaml to switch LAN discovery off, or NSELF_CI_DISCOVERY=1 to allow it"},
	{IDUpgrade, "agents at the pinned release", "Run: nself ci nodes upgrade <node> after upgrading the coordinator, or wait for the coordinator to accept the release"},
	{IDGitHub, "no persistent GitHub runner on a public repository", "Use a just-in-time GitHub runner for public repositories (provider github-jit) and remove the persistent runner"},
	{IDSeparate, "UID separation on nodes offered to a TEAM coordinator", "Enable unprivileged user namespaces on the node and re-add it, or remove it from the TEAM coordinator"},
}

// Catalog returns the checks in id order. The caller must not modify the rows.
func Catalog() []Entry { return append([]Entry(nil), catalog...) }

// Info returns the row for id; ok is false for an id this package does not own.
func Info(id string) (Entry, bool) {
	for _, e := range catalog {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}
