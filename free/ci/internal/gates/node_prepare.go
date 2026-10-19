package gates

// Purpose: gate preparation for the Node checks (P7-AUTO-16, cap:ci.gate-prep):
// a frozen install per declared package manager, `astro sync` before the
// type check and build of an Astro project, and the packageManager version pin.
// Inputs:  the planned check, the package.json of its directory and the
// lockfile found in that directory or an ancestor (a workspace member installs
// at the workspace root).
// Outputs: a nodeSpec, the shell argv that prepares then runs the check, and
// the verdict on a failed prepare.
// Constraints: every attempt runs in its own workspace copy, so prepare and
// check share ONE invocation (the Python gates do the same); nothing installs
// globally and the manager caches live inside the workspace (removed with it);
// the install log is shown only when the install fails, so a passing install
// never pushes the check's own output out of the excerpt; a lockfile that is
// out of sync fails with the manager's own message as a code failure, while a
// network, disk or missing-tool failure of the install is an infrastructure
// error (E811, never `failure`); a project without a lockfile keeps the earlier
// behaviour (no install), and POSIX shells only (Windows is out of scope, D-1029).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func init() {
	model.Register(model.Code{ID: "E811", Class: "infra", Summary: "Node gate preparation could not run as declared", Fix: "Install the package manager version that package.json pins (corepack enable), or fix the network or disk, and retry"})
}

const (
	// nodePreparedMarker records the lockfile digest an install matched; a
	// workspace whose marker equals the digest skips the install.
	nodePreparedMarker = "node_modules/.nself-ci-prepared"
	// prepareFailed is printed after the install log when the install failed.
	prepareFailed = "::nself-ci-prepare-failed::"
	prepareExit   = 97
	installLog    = ".nself-ci-install.log"
	// nodePrepareBudget is the time one invocation gets when it installs first
	// (the default is 10 minutes for the check alone).
	nodePrepareBudget = 20 * time.Minute
)

// nodeSpec is what preparation needs to know about one Node directory.
type nodeSpec struct {
	Manager, Pin string
	// LockDir is the slash path (from the repo root) of the directory that
	// holds the lockfile; "" means there is none and nothing is installed.
	LockDir string
	// Rel is the path from the working directory to LockDir ("." when equal).
	Rel string
	// Digest identifies the lockfile and manager; Prepared is true when the
	// node_modules next to the lockfile already carries that digest.
	Digest   string
	Prepared bool
	// Berry marks a Yarn 2+ lockfile (`yarn install --immutable`).
	Berry bool
	// Astro is true when package.json depends on astro.
	Astro bool
	// HasModules is true when a node_modules already sits in the working
	// directory or next to the lockfile (an install this run does not redo).
	HasModules bool
}

func nodeSpecFor(root string, p PlannedCheck) nodeSpec {
	m, _ := readNodeManifest(root, p.Workdir)
	s := nodeSpec{Rel: "."}
	s.Manager, s.Pin = nodeManager(root, p.Workdir, m)
	_, a1 := m.Dependencies["astro"]
	_, a2 := m.DevDependencies["astro"]
	s.Astro = a1 || a2
	work := path.Clean(p.Workdir)
	s.HasModules = hasDir(root, work, "node_modules")
	for dir := work; ; dir = path.Dir(dir) {
		for _, lock := range nodeLockfiles[s.Manager] {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), lock))
			if err != nil {
				continue
			}
			sum := sha256.Sum256(append(append([]byte(s.Manager+"\x00"), b...), manifestsOf(root, dir)...))
			s.LockDir, s.Digest = dir, hex.EncodeToString(sum[:])
			s.Berry = s.Manager == "yarn" && strings.Contains(string(b), "__metadata:")
			s.Rel = relUp(work, dir)
			s.HasModules = hasDir(root, work, "node_modules") || hasDir(root, dir, "node_modules")
			marker, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(nodePreparedMarker)))
			s.Prepared = strings.TrimSpace(string(marker)) == s.Digest
			return s
		}
		if dir == "." || dir == "/" || dir == "" {
			return s
		}
	}
}

// manifestsOf is every package.json and pnpm-workspace.yaml under dir (not
// inside node_modules, .git or the CI scratch): the frozen install validates
// the lockfile against all of them, so a member's edit must change the digest.
// More than maxManifests files, or an unreadable tree, returns a changing
// value, so the install is never skipped on a guess.
func manifestsOf(root, dir string) []byte {
	const maxManifests = 500
	base := filepath.Join(root, filepath.FromSlash(dir))
	var out []byte
	n := 0
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", ".nself-ci":
				return filepath.SkipDir
			}
			return nil
		}
		if name := d.Name(); name == "package.json" || name == "pnpm-workspace.yaml" {
			if n++; n > maxManifests {
				return errors.New("too many manifests")
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			rel, _ := filepath.Rel(base, p)
			out = append(append(append(out, rel...), 0), b...)
		}
		return nil
	})
	if err != nil {
		return []byte(time.Now().String())
	}
	return out
}

func hasDir(root, dir, name string) bool {
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), name))
	return err == nil && st.IsDir()
}

// relUp is the relative path from the directory work up to its ancestor dir.
func relUp(work, dir string) string {
	if work == dir {
		return "."
	}
	n := strings.Count(strings.TrimPrefix(work, dir), "/")
	if dir == "." {
		n = strings.Count(work, "/") + 1
	}
	return strings.TrimSuffix(strings.Repeat("../", n), "/")
}

// install is the frozen install command of the manager.
func (s nodeSpec) install() string {
	switch s.Manager {
	case "npm":
		return "npm ci --prefer-offline"
	case "pnpm":
		return "pnpm install --frozen-lockfile --prefer-offline"
	case "yarn":
		if s.Berry {
			return "yarn install --immutable"
		}
		return "yarn install --frozen-lockfile --prefer-offline"
	}
	return "bun install --frozen-lockfile"
}

// astroSync is the manager's way to run the project-local astro binary.
func (s nodeSpec) astroSync() string {
	switch s.Manager {
	case "npm":
		return "npm exec --no-install -- astro sync"
	case "pnpm":
		return "pnpm exec astro sync"
	case "yarn":
		return "yarn run astro sync"
	}
	return "bun run astro sync"
}

// cacheEnv keeps the manager's cache inside the workspace (removed with it).
// Only the manager's own variables: npm warns about another manager's.
func (s nodeSpec) cacheEnv() string {
	switch s.Manager {
	case "npm":
		return `export npm_config_cache="$PWD/.nself-ci/npm"`
	case "pnpm":
		return `export npm_config_store_dir="$PWD/.nself-ci/pnpm" COREPACK_ENABLE_DOWNLOAD_PROMPT=0`
	case "yarn":
		return `export YARN_CACHE_FOLDER="$PWD/.nself-ci/yarn" YARN_ENABLE_GLOBAL_CACHE=false COREPACK_ENABLE_DOWNLOAD_PROMPT=0`
	}
	return `export BUN_INSTALL_CACHE_DIR="$PWD/.nself-ci/bun"`
}

// wrap returns the argv that prepares then runs the check: run is the plain
// check command, returned unchanged when there is nothing to prepare.
func (s nodeSpec) wrap(id string, run []string) []string {
	tail := shellJoin(run)
	if id == "node.coverage" {
		tail = s.Manager + " run test -- --coverage && cat coverage/lcov.info"
	}
	if runtime.GOOS == "windows" {
		if id == "node.coverage" {
			return []string{"cmd", "/C", s.Manager + " run test -- --coverage && type coverage\\lcov.info"}
		}
		return run
	}
	needInstall := s.LockDir != "" && !s.Prepared
	// astro sync needs the dependencies: this run installs them, or they are there already
	needAstro := s.Astro && (id == "node.typecheck" || id == "node.build") && (needInstall || s.HasModules || s.LockDir != "")
	if !needInstall && !needAstro {
		if id == "node.coverage" {
			return []string{"sh", "-c", tail}
		}
		return run
	}
	script := s.cacheEnv() + "; "
	if needInstall {
		in := s.install() + " && mkdir -p node_modules && printf %s " + s.Digest + " > " + nodePreparedMarker // Yarn PnP and a project without dependencies install no node_modules
		if s.Rel != "." {
			in = "cd " + s.Rel + " && " + in
		}
		// the log is read from the working directory, so name it absolutely
		script += `LOG="$PWD/` + installLog + `"; ( ` + in + ` ) > "$LOG" 2>&1 || { head -c 100000 "$LOG"; echo; tail -c +100001 "$LOG" | tail -c 100000; echo; echo ` + prepareFailed + `; exit ` + strconv.Itoa(prepareExit) + `; }; `
	}
	if needAstro {
		script += s.astroSync() + " && "
	}
	return []string{"sh", "-c", script + tail}
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellJoin quotes each argument for sh.
func shellJoin(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if shellSafe.MatchString(a) {
			out[i] = a
		} else {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(out, " ")
}

// installs reports whether the wrapped command will run an install.
func (s nodeSpec) installs() bool {
	return runtime.GOOS != "windows" && s.LockDir != "" && !s.Prepared
}

var versionNumber = regexp.MustCompile(`\d+(\.\d+)*`)

// pinMismatch returns why the installed manager contradicts the packageManager
// pin ("" when it matches, no pin is declared, or the version is unreadable):
// the major version must be the pinned one, because lockfile formats and the
// frozen flags differ across majors.
func (s nodeSpec) pinMismatch(versionOutput string) string {
	if s.Pin == "" {
		return ""
	}
	found := versionNumber.FindString(versionOutput)
	pin := versionNumber.FindString(s.Pin)
	if found == "" || pin == "" || major(found) == major(pin) {
		return ""
	}
	return "package.json pins " + s.Manager + "@" + s.Pin + " but " + s.Manager + " " + found + " is installed"
}

func major(v string) string { return strings.SplitN(v, ".", 2)[0] }

// ends keeps the first and the last half of a long log: a manager states its
// verdict at the top (npm: "package.json and package-lock.json are not in sync")
// and the cause or the log path at the bottom.
func ends(log string, max int) string {
	if len(log) <= max {
		return log
	}
	head, tail := log[:max/2], log[len(log)-max/2:]
	return strings.ToValidUTF8(head, "") + "\n...\n" + strings.ToValidUTF8(tail, "")
}

// prepareVerdict turns the result of a failed prepare (the sentinel is in the
// output) into the check outcome. It reports whether the output was a prepare
// failure at all.
func prepareVerdict(c *model.Check, res invocation) bool {
	i := strings.LastIndex(res.output, prepareFailed)
	if i < 0 || res.exit == 0 {
		return false
	}
	log := strings.TrimSpace(res.output[:i])
	d := diagnose.ClassifyInstall(log)
	excerpt := ends(log, 1024)
	if d.Class == "infra" {
		c.Result, c.FailureClass = "error", "infra"
		c.Excerpt = "E811: dependency install failed (" + d.Signature + "); " + d.Remediation + "\n" + excerpt
		return true
	}
	c.Result, c.FailureClass = "fail", "code"
	c.Excerpt = excerpt
	return true
}
