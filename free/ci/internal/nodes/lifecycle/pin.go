package lifecycle

// Purpose: tell the admin when an agent already on the node is not the pinned release (debt D-1223).
// Inputs:  the version the installed agent reported and the default source, when it carries a pin.
// Outputs: a warning line for AddResult.Warnings; nothing when there is no pin or the versions agree.
// Constraints: advisory only. An agent already on the node is still used as it is (it is neither installed nor
//          verified here); the warning names the two repairs. Verifying an installed file needs an agent-side
//          hash report, which is not built.

// PinnedSource is an AgentSource that knows the release version it installs by default.
type PinnedSource interface {
	PinnedVersion() string
}

// pinWarning compares the installed agent version with the pin of the default source.
func pinWarning(id, installed string) []string {
	src := ext.defaultSource()
	p, ok := src.(PinnedSource)
	if !ok || p.PinnedVersion() == "" || p.PinnedVersion() == installed {
		return nil
	}
	return []string{"agent " + installed + " is already on the node and differs from the pinned release " + p.PinnedVersion() +
		"; it was used as it is and not verified. Reinstall with --agent-binary <path> or run `nself ci nodes upgrade " + id + "`"}
}
