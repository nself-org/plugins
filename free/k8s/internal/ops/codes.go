// Purpose: the typed errors of backup and restore and their codes E730 to E738.
//
// Inputs: a failure inside internal/ops (kubectl, psql, pg_dump, pg_restore, the
// backup file or its metadata).
//
// Outputs: *Error with a code, a class (user or infra), a fixed message, a fixed
// remediation and an optional bounded cause. cmd maps it to an output.ErrorDetail.
//
// Constraints: messages, remediations and causes are fixed text: a step, an exit status, a
// marker state, a count of error lines, or table names (E737). Database-provided error
// text is kept apart in Detail and never reaches the envelope; HumanDetail turns it into
// allowlisted category words for human-mode stderr.
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
	"unicode/utf8"
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

// maxHuman bounds the error category words shown in human mode.
const maxHuman = 160

// Error is a coded failure of backup or restore.
type Error struct {
	Code        string
	Class       string
	Message     string
	Remediation string
	Cause       string
	// Detail is the first error line the tool printed. It is database-provided text and
	// may carry a row value, so it is never serialized: only HumanDetail (category words)
	// reaches human-mode stderr.
	Detail string
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

// errChangedDuringRestore is E735 raised after pg_restore ran: the file's hash while
// streaming differs from the recorded one.
func errChangedDuringRestore() *Error {
	return newErr(CodeChecksum, ClassInfra, "the file changed while it was restored: pg_restore ran and the database state is not verified",
		"restore again from an unchanged copy of the backup", "")
}

func errRestore(cause, detail string) *Error {
	e := newErr(CodeRestoreFailed, ClassInfra, "pg_restore reported errors: the database state is not verified",
		"fix the cause and run the restore again; pg_restore's own output was not kept", cause)
	e.Detail = detail
	return e
}

// errRestoreShort is E736: pg_restore finished but did not consume the whole archive.
func errRestoreShort() *Error {
	return newErr(CodeRestoreFailed, ClassInfra, "pg_restore did not read the whole archive; the database state is not verified",
		"restore again; check the connection to the cluster", "")
}

// humanCategories is the allowlist HumanDetail draws from: a marker in the tool's first
// error line (matched lower-case) and the fixed words shown for it. Nothing of the line
// itself is ever shown, because a trigger or check function can put any value, quoted or
// not, in the text of its own exception.
var humanCategories = []struct{ marker, shown string }{
	{"could not execute query", "a statement failed"},
	{"copy failed", "COPY failed"},
	{"violates not-null constraint", "not-null constraint violated"},
	{"violates foreign key constraint", "foreign key violated"},
	{"violates check constraint", "check constraint violated"},
	{"violates unique constraint", "unique constraint violated"},
	{"duplicate key value", "duplicate key"},
	{"invalid input syntax", "invalid input value"},
	{"permission denied", "permission denied"},
	{"must be owner", "not the owner"},
	{"already exists", "object already exists"},
	{"does not exist", "object does not exist"},
	{"syntax error", "SQL syntax error"},
	{"unsupported version", "unsupported archive version"},
	{"out of memory", "out of memory"},
	{"no space left", "no space left on device"},
	{"could not open", "a file could not be opened"},
	{"could not connect", "connection failed"},
	{"connection to server", "connection failed"},
}

// HumanDetail names the failure category of the first error line for a person: only the
// fixed words of humanCategories whose marker the line holds, never any text of the line
// (so no value, quoted or not, from the database), at most maxHuman bytes. A line with no
// known marker is reported as withheld. "" when there is no line.
func (e *Error) HumanDetail() string {
	line := strings.ToLower(e.Detail)
	if strings.TrimSpace(line) == "" {
		return ""
	}
	var shown []string
	seen := map[string]bool{}
	for _, c := range humanCategories {
		if strings.Contains(line, c.marker) && !seen[c.shown] {
			seen[c.shown] = true
			shown = append(shown, c.shown)
		}
	}
	if len(shown) == 0 {
		return "an error line outside the known categories (text withheld)"
	}
	out := strings.Join(shown, "; ")
	if len(out) > maxHuman {
		out = out[:maxHuman]
		for len(out) > 0 && !utf8.ValidString(out) {
			out = out[:len(out)-1]
		}
	}
	return out
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
		"set PROJECT_NAME in the environment or the project .env (letters, digits, '.', '_', '-')", cause)
}

// errTargetWith is E738 with its own remediation.
func errTargetWith(msg, remediation string) *Error {
	return newErr(CodeTarget, ClassUser, msg, remediation, "")
}

func errExists() *Error {
	return errTargetWith("the backup file already exists: nothing was written",
		"choose another --output or --tag, or move the existing file away; a backup is never overwritten")
}
