// Purpose: the typed errors of backup and restore and their codes E730 to E738.
//
// Inputs: a failure inside internal/ops (kubectl, psql, pg_dump, pg_restore, the
// backup file or its metadata).
//
// Outputs: *Error with a code, a class (user or infra), a fixed message, a fixed
// remediation and an optional bounded cause. cmd maps it to an output.ErrorDetail.
//
// Constraints: messages and remediations are fixed text. Cause is limited to an
// exit status, a table name or one line of the tool's own error text (at most
// maxCause bytes): never an environment value, a password or SQL result data.
//
//	E730 kubectl not on PATH                                     infra
//	E731 postgres pod not found, not unique or not reachable     user
//	E732 backup stream failed                                    infra
//	E733 fingerprint query failed                                infra
//	E734 backup file or its metadata missing, unreadable, invalid user
//	E735 dump does not match its recorded checksum               infra
//	E736 pg_restore failed                                       infra
//	E737 post-restore fingerprints differ from the backup's      infra
//	E738 backup target unresolved (name, tag, existing output)   user
package ops

import (
	"fmt"
	"strings"
)

// Error codes owned by this package.
const (
	CodeKubectlMissing = "E730"
	CodePodNotFound    = "E731"
	CodeBackupFailed   = "E732"
	CodeFingerprint    = "E733"
	CodeBackupInvalid  = "E734"
	CodeChecksum       = "E735"
	CodeRestoreFailed  = "E736"
	CodeMismatch       = "E737"
	CodeTarget         = "E738"
)

// Classes of an Error.
const (
	ClassUser  = "user"
	ClassInfra = "infra"
)

// maxCause bounds the cause text of an Error.
const maxCause = 1500

// Error is a coded failure of backup or restore.
type Error struct {
	Code        string
	Class       string
	Message     string
	Remediation string
	Cause       string
}

func (e *Error) Error() string {
	s := e.Code + " " + e.Message
	if e.Cause != "" {
		s += ": " + e.Cause
	}
	return s
}

// newErr builds an Error and bounds its cause to one clean line.
func newErr(code, class, msg, remediation, cause string) *Error {
	return &Error{Code: code, Class: class, Message: msg, Remediation: remediation, Cause: oneLine(cause, maxCause)}
}

// oneLine trims s, folds line breaks and cuts it to at most n bytes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func errKubectlMissing() *Error {
	return newErr(CodeKubectlMissing, ClassInfra, "the kubectl binary was not found on PATH",
		"install kubectl from https://kubernetes.io/docs/tasks/tools/", "")
}

func errPod(msg, cause string) *Error {
	return newErr(CodePodNotFound, ClassUser, msg,
		"check --cluster, --namespace and --release; the release needs one running postgres pod", cause)
}

func errBackup(cause string) *Error {
	return newErr(CodeBackupFailed, ClassInfra, "the backup stream failed; no backup file was kept",
		"check the postgres pod and the free disk space, then run the backup again", cause)
}

func errFingerprint(cause string) *Error {
	return newErr(CodeFingerprint, ClassInfra, "the table fingerprints could not be read",
		"check that the postgres pod is ready and accepts connections", cause)
}

func errInvalid(msg, cause string) *Error {
	return newErr(CodeBackupInvalid, ClassUser, msg,
		"restore a backup made by nself k8s backup: it has a metadata file under .meta next to it", cause)
}

func errChecksum(cause string) *Error {
	return newErr(CodeChecksum, ClassInfra, "the dump does not match its recorded checksum: nothing was restored",
		"the file changed after the backup; restore from another copy", cause)
}

func errRestore(cause string) *Error {
	return newErr(CodeRestoreFailed, ClassInfra, "pg_restore reported errors: the database state is not verified",
		"fix the cause and run the restore again; pg_restore's own output was not kept", cause)
}

func errMismatch(diffs []Diff) *Error {
	lines := make([]string, 0, maxDiffs)
	for i, d := range diffs {
		if i == maxDiffs {
			lines = append(lines, fmt.Sprintf("and %d more", len(diffs)-maxDiffs))
			break
		}
		lines = append(lines, d.String())
	}
	return newErr(CodeMismatch, ClassInfra, "the restored data does not match the backup's fingerprints",
		"stop every writer of the database and run the restore again", strings.Join(lines, "; "))
}

func errTarget(msg, cause string) *Error {
	return newErr(CodeTarget, ClassUser, msg,
		"set PROJECT_NAME in the environment or the project .env, and use a new file name", cause)
}
