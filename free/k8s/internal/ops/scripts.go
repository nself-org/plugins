// Purpose: the fixed commands that run inside the postgres pod.
//
// Inputs: a snapshot id for pg_dump (the only variable part).
//
// Outputs: argv slices for Exec: an interactive psql, pg_dump of one snapshot and
// pg_restore reading the dump on stdin.
//
// Constraints: every command is `sh -c '<fixed script>'`. The script reads the
// connection settings from the pod's own environment (POSTGRES_USER, POSTGRES_DB,
// POSTGRES_PASSWORD, as the postgres image and the chart define them), so no
// value is ever on an argv here or in kubectl's. SQL goes on stdin, never argv.
// The snapshot id is validated against the form pg_export_snapshot returns and
// is passed as a positional parameter, never spliced into the script text.
package ops

import "regexp"

// podPrelude exports the password for libpq and resolves user and database with
// the postgres image's own defaults.
const podPrelude = `export PGPASSWORD="$POSTGRES_PASSWORD"; U="${POSTGRES_USER:-postgres}"; D="${POSTGRES_DB:-$U}"; `

var (
	psqlArgv      = []string{"sh", "-c", podPrelude + `exec psql -X -q -A -t -v ON_ERROR_STOP=1 -U "$U" -d "$D"`}
	pgRestoreArgv = []string{"sh", "-c", podPrelude + `exec pg_restore -U "$U" -d "$D" --clean --if-exists`}
)

// snapshotRe is the form of a snapshot id: 00000003-0000001B-1.
var snapshotRe = regexp.MustCompile(`^[0-9A-F]+-[0-9A-F]+(-[0-9]+)?$`)

// pgDumpArgv returns the pg_dump command for one exported snapshot, or false when
// the id is not a snapshot id.
func pgDumpArgv(snapshot string) ([]string, bool) {
	if !snapshotRe.MatchString(snapshot) {
		return nil, false
	}
	return []string{"sh", "-c", podPrelude + `exec pg_dump -U "$U" -d "$D" -Fc --snapshot="$1"`, "sh", snapshot}, true
}
