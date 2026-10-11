// Purpose: carry the postgres init scripts to the cluster (debt D-0308). A
// compose bind mount whose target is /docker-entrypoint-initdb.d is mapped
// instead of listed notMapped: the file NAMES go to values.yaml
// (Service.InitDB) and the file CONTENTS go to secrets.yaml (Secrets.InitDB),
// because an init script may hold a password. The chart renders them as one
// Secret per service and mounts it at /docker-entrypoint-initdb.d.
//
// Inputs: one resolved compose service, a SourceFS that opens the bind source
// directory (default: the real disk, tests pass a fake).
//
// Outputs: Service.InitDB, Secrets.InitDB, extra Service.NotMapped entries
// "initdb:<name>" for what is skipped.
//
// Constraints: the directory is read non-recursively. Carried: regular files
// named *.sql, *.sql.gz and *.sh, sorted by name. A .sql.gz script is stored as
// base64 text (a Secret carries it as binary data); the others must be valid
// UTF-8. Dot files are ignored (the postgres entrypoint glob never sees them);
// every other entry (directory, symlink, other extension, unusual name) is
// listed in notMapped. A missing source directory is an empty list plus
// "initdb:source-missing". The scripts of one service may total at most 900 KiB
// (a Secret holds 1 MiB); the error names the file and service, never content.
package values

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// initDBTarget is the container path the postgres image runs scripts from.
	initDBTarget = "/docker-entrypoint-initdb.d"
	// initDBLimit is the most the scripts of one service may total (bytes, as
	// stored in the Secret).
	initDBLimit = 900 << 10
)

// initDBName is what a Kubernetes Secret accepts as a data key.
var initDBName = regexp.MustCompile(`^[-._a-zA-Z0-9]+$`)

// SourceFS opens the directory a compose bind mount reads from.
type SourceFS func(source string) (fs.FS, error)

// osSource is the production SourceFS: the real directory.
func osSource(source string) (fs.FS, error) { return os.DirFS(source), nil }

// MapOptions tunes Map; the zero value is the production behaviour.
type MapOptions struct {
	// InitFS opens init-script directories (default: the real disk).
	InitFS SourceFS
}

func (o MapOptions) initFS() SourceFS {
	if o.InitFS == nil {
		return osSource
	}
	return o.InitFS
}

// isInitDB reports whether mt is the postgres init-script bind mount.
func isInitDB(mt ComposeMount) bool {
	return mt.Type == "bind" && path.Clean(mt.Target) == initDBTarget
}

// addInitDB maps the init-script mount of cs, if it has one, onto svc and sec.
func addInitDB(name string, cs ComposeSvc, svc *Service, sec *Secrets, open SourceFS) error {
	for _, mt := range cs.Volumes {
		if !isInitDB(mt) {
			continue
		}
		fsys, err := open(mt.Source)
		if err != nil {
			return fmt.Errorf("service %s: initdb source directory cannot be opened: %w", name, err)
		}
		names, files, skipped, err := readInitDir(name, fsys)
		if err != nil {
			return err
		}
		svc.InitDB = names
		svc.NotMapped = append(svc.NotMapped, skipped...)
		sort.Strings(svc.NotMapped)
		if len(files) > 0 {
			if sec.InitDB == nil {
				sec.InitDB = map[string]map[string]string{}
			}
			sec.InitDB[name] = files
		}
	}
	return nil
}

// readInitDir lists and reads the carried scripts of one directory.
func readInitDir(service string, fsys fs.FS) (names []string, files map[string]string, skipped []string, err error) {
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, []string{"initdb:source-missing"}, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("service %s: initdb source directory cannot be read: %w", service, err)
	}
	files = map[string]string{}
	total := 0
	for _, e := range entries { // fs.ReadDir sorts by name
		n := e.Name()
		switch {
		case strings.HasPrefix(n, "."):
			continue
		case !e.Type().IsRegular() || !carriedExt(n) || !initDBName.MatchString(n):
			skipped = append(skipped, "initdb:"+n)
			continue
		}
		data, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("service %s: initdb file %s cannot be read: %w", service, n, err)
		}
		val, err := initDBValue(service, n, data)
		if err != nil {
			return nil, nil, nil, err
		}
		if total += len(val); total > initDBLimit {
			return nil, nil, nil, fmt.Errorf("service %s: initdb file %s takes the scripts over %d KiB (a Kubernetes Secret holds 1 MiB)",
				service, n, initDBLimit>>10)
		}
		names = append(names, n)
		files[n] = val
	}
	return names, files, skipped, nil
}

func carriedExt(n string) bool {
	return strings.HasSuffix(n, ".sql") || strings.HasSuffix(n, ".sql.gz") || strings.HasSuffix(n, ".sh")
}

// initDBValue is the secrets.yaml text of one script.
func initDBValue(service, name string, data []byte) (string, error) {
	if strings.HasSuffix(name, ".gz") {
		return base64.StdEncoding.EncodeToString(data), nil
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("service %s: initdb file %s is not valid UTF-8 text", service, name)
	}
	return string(data), nil
}
