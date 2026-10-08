// Package exec runs local CI attempts in private workspaces. On Unix, cancellation
// signals the process group and observed descendants, including children that
// create a new session. Linux also makes the executor a child subreaper so an
// orphaned grandchild remains attached to the executor for cleanup. On Darwin,
// a process that double-forked to launchd before cancellation cannot be
// reliably associated with its original attempt and may remain running.
package exec
