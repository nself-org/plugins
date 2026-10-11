// Purpose: where a backup goes and what it is called, in the core backup layout.
//
// Inputs: the project directory, PROJECT_NAME and BACKUP_DIR (process environment
// first, else the KEY=value lines of the project .env), --output, --tag, a clock.
//
// Outputs: the absolute path of the dump file, or E738.
//
// Constraints: core layout: <dir>/<PROJECT_NAME>_full_<YYYYMMDD_HHMMSS>[_<tag>].dump,
// dir = BACKUP_DIR (default ./backups, relative to the project dir). --output names
// the file exactly; its directory is created 0700. PROJECT_NAME and the tag are
// limited to [A-Za-z0-9._-] (first character alphanumeric for the name) so neither
// can carry a path separator. An existing file is never replaced (E738).
package ops

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	tagRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// TargetOptions is the input of ResolveTarget.
type TargetOptions struct {
	ProjectDir string
	Output     string // --output; empty means the backup dir
	Tag        string // --tag
	Getenv     func(string) string
	Now        time.Time
}

// ResolveTarget returns the path of the dump to write and the project name
// recorded in its metadata (empty with --output when PROJECT_NAME is not usable).
func ResolveTarget(o TargetOptions) (path, project string, err error) {
	if o.Tag != "" && !tagRe.MatchString(o.Tag) {
		return "", "", errTarget("--tag may only contain letters, digits, '.', '_' and '-'", "")
	}
	dotenv := readDotEnv(filepath.Join(o.ProjectDir, ".env"))
	get := func(k string) string {
		if o.Getenv != nil {
			if v := o.Getenv(k); v != "" {
				return v
			}
		}
		return dotenv[k]
	}
	name := get("PROJECT_NAME")
	if o.Output != "" {
		if strings.HasSuffix(o.Output, "/") {
			return "", "", errTarget("--output must name a file, not a directory", "")
		}
		if !nameRe.MatchString(name) {
			name = ""
		}
		p, err := absUnder(o.ProjectDir, o.Output)
		return p, name, err
	}
	if !nameRe.MatchString(name) {
		return "", "", errTarget("PROJECT_NAME is not set or not a plain name (letters, digits, '.', '_', '-')", "")
	}
	dir := get("BACKUP_DIR")
	if dir == "" {
		dir = "backups"
	}
	file := name + "_full_" + o.Now.UTC().Format("20060102_150405")
	if o.Tag != "" {
		file += "_" + o.Tag
	}
	p, err := absUnder(o.ProjectDir, filepath.Join(dir, file+".dump"))
	return p, name, err
}

// absUnder makes p absolute, relative paths being relative to base.
func absUnder(base, p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", errTarget("the backup path cannot be resolved", "")
	}
	return abs, nil
}

// readDotEnv parses KEY=value lines (optional "export ", one pair of quotes
// stripped). A missing or unreadable file is an empty map.
func readDotEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}
