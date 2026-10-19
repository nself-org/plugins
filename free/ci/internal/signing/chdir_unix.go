//go:build unix

package signing

// Purpose: run a call in a chosen directory on unix (see stdio.go).
// Inputs: a directory and an argv. Outputs: the argv wrapped in `sh -c cd`.
// Constraints: the funnel has no working-directory field, so a missing
// directory exits 126 from the wrapper; nothing is written to disk.

// chdirScript changes directory, then replaces the shell with the call.
const chdirScript = `d=$1; shift
cd "$d" || exit 126
exec "$@"`

func inDir(dir string, argv []string) ([]string, error) {
	return append([]string{"/bin/sh", "-c", chdirScript, "sh", dir}, argv...), nil
}
