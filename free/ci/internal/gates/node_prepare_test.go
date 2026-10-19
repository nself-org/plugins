package gates

// Purpose: P7-AUTO-16 Node preparation through the REAL path: the checks are
// planned from detector facts (Registry.Plan), the planned argv runs in the
// executor like a run's job does, and a failed attempt is classified by
// diagnose.Classify like internal/run does. Fake package managers sit on a
// private PATH.
// Constraints: POSIX only.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var lockfileOf = map[string]string{"npm": "package-lock.json", "pnpm": "pnpm-lock.yaml", "yarn": "yarn.lock", "bun": "bun.lock"}

// nodeHost is a repository root plus a private PATH holding only fake managers.
type nodeHost struct {
	t    *testing.T
	root string
	bin  string
}

func newNodeHost(t *testing.T) *nodeHost {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	h := &nodeHost{t: t, root: t.TempDir(), bin: t.TempDir()}
	for _, tool := range []string{"sh", "ps", "tail", "head", "cat", "mkdir", "sed", "grep", "cut", "wc", "rm", "basename", "awk", "touch", "ln"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(p, filepath.Join(h.bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", h.bin)
	return h
}

// manager installs a fake manager that answers --version and then runs body.
func (h *nodeHost) manager(name, version, body string) {
	h.t.Helper()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo " + version + "; exit 0; fi\n" + body
	if err := os.WriteFile(filepath.Join(h.bin, name), []byte(script), 0o700); err != nil {
		h.t.Fatal(err)
	}
}

func (h *nodeHost) write(rel, content string) {
	h.t.Helper()
	p := filepath.Join(h.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// plan returns the planned check id in workdir, from detector facts like `nself ci` does.
func (h *nodeHost) plan(id, workdir string) PlannedCheck {
	h.t.Helper()
	reg := DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(h.root), reg.Has)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, p := range reg.Plan(facts, Options{Config: model.PipelineConfig{Checks: map[string]model.CheckDef{"node.coverage": {CoverageFloor: intPtr(1)}}}}) {
		if p.Def.ID == id && filepath.ToSlash(filepath.Clean(p.Workdir)) == workdir {
			return p
		}
	}
	h.t.Fatalf("%s not planned in %s", id, workdir)
	return PlannedCheck{}
}

func intPtr(n int) *int { return &n }

// result is one planned check run, as a run's job: exit code, output, diagnosis.
type result struct {
	exit int
	out  string
	diag diagnose.Diagnosis
}

func (h *nodeHost) run(id, workdir string) result {
	h.t.Helper()
	p := h.plan(id, workdir)
	var out strings.Builder
	res := (&ciexec.Executor{}).Run(context.Background(), ciexec.JobSpec{AttemptID: "node-" + strings.ReplaceAll(id, ".", "-"), CoordinatorID: "coord",
		Job: model.Job{Kind: "quality", Workdir: workdir, Isolation: "process"}, Command: p.Command(), SourceDir: h.root, Home: h.t.TempDir(),
		Output: func(b []byte) { out.Write(b) }})
	r := result{exit: res.ExitCode, out: strings.TrimSpace(out.String())}
	if res.Err != nil {
		h.t.Fatalf("executor: %v", res.Err)
	}
	if res.Attempt.State != "passed" {
		tail := out.String()
		if len(tail) > 8<<10 { // a run keeps the last 8 KiB
			tail = tail[len(tail)-8<<10:]
		}
		r.diag = diagnose.Classify(diagnose.Input{ExecClass: res.Attempt.FailureClass, ExitCode: res.ExitCode, Tail: tail})
	}
	return r
}

func manifest(pm string, extra string) string {
	field := ""
	if pm != "" {
		field = `"packageManager":"` + pm + `",`
	}
	return `{` + field + extra + `"scripts":{"lint":"x","typecheck":"x","test":"x","build":"x"}}`
}

// passing is a fake manager whose install succeeds noisily and whose runs print one line.
const passing = `case "$1" in
ci|install) echo "INSTALL-NOISE $*"; mkdir -p node_modules;;
run) echo "RUN $2";;
exec) echo "EXEC $*";;
esac
`

func TestNodePlanSetsManagerToolAndArgv(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.write("packages/a/package.json", manifest("", ""))
	h.write("tools/b/package.json", manifest("npm@10.0.0", ""))
	for dir, want := range map[string]string{".": "pnpm", "packages/a": "pnpm", "tools/b": "npm"} {
		p := h.plan("node.test", dir)
		if p.ToolName() != want || p.PlacementTool() != want || len(p.Command()) != 3 || p.Command()[0] != "sh" {
			t.Errorf("%s: tool %q argv %q, want manager %s in an sh script", dir, p.ToolName(), p.Command(), want)
		}
		if !strings.Contains(p.Command()[2], "exec "+want+" run test") {
			t.Errorf("%s: %s", dir, p.Command()[2])
		}
	}
}

func TestNodePrepareFrozenPerManager(t *testing.T) {
	want := map[string]string{
		"npm":  "ARGS ci --prefer-offline",
		"pnpm": "ARGS install --frozen-lockfile --prefer-offline",
		"yarn": "ARGS install --frozen-lockfile --prefer-offline",
		"bun":  "ARGS install --frozen-lockfile",
	}
	for pm, line := range want {
		t.Run(pm, func(t *testing.T) {
			h := newNodeHost(t)
			h.write("package.json", manifest(pm+"@9.0.0", ""))
			h.write(lockfileOf[pm], "# lock\n")
			h.manager(pm, "9.0.0", `echo "ARGS $*"; exit 1`)
			r := h.run("node.test", ".")
			if r.exit != 97 || r.diag.Class != "code" || !strings.HasPrefix(r.out, line) {
				t.Fatalf("a failed install is a code failure: %+v", r)
			}
		})
	}
	t.Run("yarn berry", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("yarn@4.1.0", ""))
		h.write("yarn.lock", "__metadata:\n  version: 8\n")
		h.manager("yarn", "4.1.0", `echo "ARGS $*"; exit 1`)
		if r := h.run("node.test", "."); !strings.HasPrefix(r.out, "ARGS install --immutable") {
			t.Fatalf("%+v", r)
		}
	})
}

func TestNodePrepareRunsThenPasses(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", passing)
	r := h.run("node.test", ".")
	if r.exit != 0 || r.out != "RUN test" {
		t.Fatalf("a passing install must not reach the check's output: %+v", r)
	}
}

func TestNodePrepareInstallWithoutNodeModules(t *testing.T) {
	// Yarn PnP and a project without dependencies install no node_modules
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", `case "$1" in ci) echo "up to date";; run) echo "RUN $2";; esac
`)
	if r := h.run("node.test", "."); r.exit != 0 || r.out != "RUN test" {
		t.Fatalf("%+v", r)
	}
}

func TestNodeInstalledTreeIsUsedAsItIs(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.write("node_modules/.keep", "")
	h.manager("pnpm", "9.1.0", `case "$1" in install) echo "REINSTALLED"; exit 1;; run) echo "RUN $2";; esac
`)
	if r := h.run("node.test", "."); r.exit != 0 || r.out != "RUN test" {
		t.Fatalf("a developer's node_modules must not be installed over: %+v", r)
	}
}

func TestNodeWithoutLockfileKeepsEarlierBehaviour(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.manager("npm", "10.2.4", `case "$1" in ci) echo "UNEXPECTED"; exit 1;; run) echo "RUN $2";; esac
`)
	if r := h.run("node.test", "."); r.exit != 0 || r.out != "RUN test" {
		t.Fatalf("%+v", r)
	}
}

func TestNodePrepareWorkspaceMemberInstallsAtTheLockfile(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", `{"private":true,"packageManager":"pnpm@9.1.0"}`)
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.write("packages/a/package.json", manifest("", ""))
	h.manager("pnpm", "9.1.0", `case "$1" in install) echo "CWD $(basename "$PWD")"; mkdir -p node_modules;; run) echo "RUN $2 in $(basename "$PWD")";; esac
`)
	r := h.run("node.test", "packages/a")
	if r.exit != 0 || r.out != "RUN test in a" {
		t.Fatalf("%+v", r)
	}
	// the install ran at the root: make it fail to see where
	h.manager("pnpm", "9.1.0", `echo "CWD $(basename "$PWD")"; exit 1`)
	if r := h.run("node.test", "packages/a"); strings.HasPrefix(r.out, "CWD a") || !strings.HasPrefix(r.out, "CWD ") {
		t.Fatalf("the member must install at the workspace root: %+v", r)
	}
}

func TestNodePrepareLockfileOutOfSyncIsCode(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.manager("pnpm", "9.1.0", `echo ' ERR_PNPM_OUTDATED_LOCKFILE  Cannot install with "frozen-lockfile" because pnpm-lock.yaml is not up to date'; exit 1`)
	r := h.run("node.build", ".")
	if r.diag.Class != "code" || !strings.Contains(r.out, "ERR_PNPM_OUTDATED_LOCKFILE") {
		t.Fatalf("%+v", r)
	}
}

func TestNodePrepareInstallFailureClasses(t *testing.T) {
	infra := map[string]string{
		"npm enotfound":   "npm error code ENOTFOUND\nnpm error syscall getaddrinfo\nnpm error errno ENOTFOUND\nnpm error network request to https://registry.npmjs.org/x failed, reason: getaddrinfo ENOTFOUND registry.npmjs.org\nnpm error A complete log of this run can be found in: /x/1.log",
		"npm etimedout":   "npm error code ETIMEDOUT\nnpm error network request to https://registry.npmjs.org/x failed, reason: connect ETIMEDOUT 1.2.3.4:443\nnpm error log",
		"pnpm meta fetch": " ERR_PNPM_META_FETCH_FAIL  GET https://registry.npmjs.org/x: Client network socket disconnected\nThis error happened while installing the dependencies of x",
		"yarn network":    "info There appears to be trouble with your network connection. Retrying...\nerror An unexpected error occurred: \"https://registry.yarnpkg.com/x: getaddrinfo ENOTFOUND registry.yarnpkg.com\".",
		"disk":            "npm error code ENOSPC\nnpm error nospc",
		"warn then real":  "npm warn ECONNRESET retrying\nnpm warn ECONNRESET retrying\nnpm error code ENOSPC\nnpm error log",
	}
	code := map[string]string{
		"out of sync":    "npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.\nnpm error Missing: x@1.0.0 from lock file",
		"sync after one": "npm warn ECONNRESET retrying\nERR_PNPM_OUTDATED_LOCKFILE  Cannot install with \"frozen-lockfile\" because pnpm-lock.yaml is not up to date",
		"berry sync":     "YN0028: The lockfile would have been modified by this install, which is explicitly forbidden.",
		"not found":      " ERR_PNPM_FETCH_404  GET https://registry.npmjs.org/nope: Not Found - 404",
		"npm 404":        "npm error code E404\nnpm error 404 Not Found - GET https://registry.npmjs.org/nope - Not found",
		"postinstall":    "> app@1.0.0 postinstall\n--- FAIL: TestRetry (ETIMEDOUT expected)\nFAIL\nnpm error code 1",
		"tool missing":   "sh: 1: tsc: not found\nnpm error code 127",
		"local database": "> app@1.0.0 postinstall\n> node scripts/migrate.js\nError: connect ECONNREFUSED 127.0.0.1:5432\nnpm error command failed",
		"assertion":      "AssertionError [ERR_ASSERTION]: expected ECONNREFUSED to be handled\nnpm error code 1",
		"unknown":        "something odd happened",
	}
	for want, set := range map[string]map[string]string{"infra": infra, "code": code} {
		for name, msg := range set {
			t.Run(want+"/"+name, func(t *testing.T) {
				h := newNodeHost(t)
				h.write("package.json", manifest("npm@10.2.4", ""))
				h.write("package-lock.json", "{}")
				h.manager("npm", "10.2.4", "cat <<'EOF'\n"+msg+"\nEOF\nexit 1\n")
				r := h.run("node.lint", ".")
				if r.exit != 97 || string(r.diag.Class) != want {
					t.Fatalf("want %s: %+v", want, r)
				}
				if want == "infra" && (!strings.HasPrefix(r.diag.Remediation, "E811:") || !r.diag.Retryable) {
					t.Fatalf("an infrastructure install failure is E811 and retryable: %+v", r.diag)
				}
			})
		}
	}
}

func TestNodePrepareGuardsReadTheMiddleOfALongLog(t *testing.T) {
	for name, lines := range map[string]int{"6 KB": 40, "350 KB": 2200} {
		t.Run(name, func(t *testing.T) {
			h := newNodeHost(t)
			h.write("package.json", manifest("npm@10.2.4", ""))
			h.write("package-lock.json", "{}")
			// the assertion sits far from both ends of the log, then a transient reset ends it
			body := `pad() { i=0; while [ $i -lt ` + strconv.Itoa(lines) + ` ]; do echo "npm info padding line number $i of the $1 part of this install log, 80 bytes....."; i=$((i+1)); done; }
pad head
echo "AssertionError [ERR_ASSERTION]: postinstall self-check"
i=0; while [ $i -lt 40 ]; do echo "npm warn cleanup ENOSPC lockfile 404 localhost $i, a keyword-heavy warning that must not evict the assertion above"; i=$((i+1)); done
pad tail
echo "npm error code ECONNRESET"
exit 1
`
			h.manager("npm", "10.2.4", body)
			r := h.run("node.test", ".")
			if r.exit != 97 || r.diag.Class != "code" {
				t.Fatalf("an assertion in the middle of the log must keep it code: %+v (output %d bytes)", r.diag, len(r.out))
			}
			if len(r.out) > 8<<10 {
				t.Fatalf("the failure report must fit the 8 KiB a run keeps: %d bytes", len(r.out))
			}
		})
	}
}

func TestPackageManagerPin(t *testing.T) {
	t.Run("a mismatch is E811 and not retried", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0+sha512.abc", ""))
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		h.manager("pnpm", "8.15.0", passing)
		r := h.run("node.test", ".")
		if r.exit != 98 || r.diag.Class != "infra" || r.diag.Retryable || !strings.HasPrefix(r.diag.Remediation, "E811:") || !strings.Contains(r.out, "pnpm 9.x but pnpm 8 is installed") {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("the same major runs", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		h.manager("pnpm", "9.12.3", passing)
		if r := h.run("node.test", "."); r.exit != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("the version probe never lets corepack bootstrap the pinned manager", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		// a corepack shim would download the pinned manager; with the network off it answers from its cache (or fails, and the pin goes unchecked)
		h.manager("pnpm", `$(if [ "$COREPACK_ENABLE_NETWORK" = 0 ]; then echo 9.1.0; else echo 0.0.1; fi)`, passing)
		if r := h.run("node.test", "."); r.exit != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("an unreadable version is no mismatch", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		h.manager("pnpm", "unknown", passing)
		if r := h.run("node.test", "."); r.exit != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("no install, no pin check", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("npm@10.0.0", ""))
		h.manager("npm", "11.0.0", passing)
		if r := h.run("node.test", "."); r.exit != 0 || r.out != "RUN test" {
			t.Fatalf("a project that installs nothing must not fail on the manager's major: %+v", r)
		}
	})
	t.Run("a member is held to the root pin", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", `{"private":true,"packageManager":"pnpm@9.1.0"}`)
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		h.write("packages/a/package.json", manifest("", ""))
		h.manager("pnpm", "8.0.0", passing)
		if r := h.run("node.test", "packages/a"); r.exit != 98 {
			t.Fatalf("%+v", r)
		}
	})
}

func TestMissingManagerE614(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	p := h.plan("node.test", ".")
	if p.ToolName() != "pnpm" || p.PlacementTool() != "pnpm" || p.Def.Remediation != "Install the declared Node package manager" {
		t.Fatalf("the declared manager is the planned tool, so a host without it is refused at placement with E614: %+v", p)
	}
	// the standalone runner agrees: required and missing is error/infra with E614, never a failure
	d, _ := DefaultRegistry().Get("node.test")
	c := (Runner{Executor: &ciexec.Executor{}, Home: t.TempDir()}).Run(context.Background(), h.root, PlannedCheck{Def: d, Required: true, Workdir: "."}, model.PipelineConfig{})
	if c.Result != "error" || c.FailureClass != "infra" || !strings.Contains(c.Excerpt, "E614") {
		t.Fatalf("%+v", c)
	}
}

func TestAstroSyncWhenDeclared(t *testing.T) {
	astro := `"devDependencies":{"astro":"^5.0.0"},`
	for pm, sync := range map[string]string{"npm": "npm exec --no-install -- astro sync", "pnpm": "pnpm exec astro sync", "yarn": "yarn run astro sync", "bun": "bun run astro sync"} {
		for _, id := range []string{"node.typecheck", "node.build"} {
			if got := nodeScript(pm, id, pm+" run x", "."); !strings.Contains(got, sync+" || exit $?") || strings.Index(got, sync) > strings.Index(got, "exec "+pm+" run x") {
				t.Errorf("%s %s: %s", pm, id, got)
			}
		}
		for _, id := range []string{"node.lint", "node.test"} {
			if got := nodeScript(pm, id, pm+" run x", "."); strings.Contains(got, "astro") {
				t.Errorf("%s %s must not sync", pm, id)
			}
		}
	}
	t.Run("runs before the check", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", astro))
		h.write("node_modules/.keep", "") // installed already
		h.manager("pnpm", "9.1.0", passing)
		if r := h.run("node.typecheck", "."); r.exit != 0 || r.out != "EXEC exec astro sync\nRUN typecheck" {
			t.Fatalf("%+v", r)
		}
		if r := h.run("node.lint", "."); r.out != "RUN lint" {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("after the install", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", astro))
		h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
		h.manager("pnpm", "9.1.0", passing)
		if r := h.run("node.build", "."); r.exit != 0 || r.out != "EXEC exec astro sync\nRUN build" {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("not declared, not run", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.write("node_modules/.keep", "")
		h.manager("pnpm", "9.1.0", passing)
		if r := h.run("node.typecheck", "."); r.out != "RUN typecheck" {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("nothing installed, nothing to sync against", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", astro))
		h.manager("pnpm", "9.1.0", passing)
		if r := h.run("node.build", "."); r.out != "RUN build" {
			t.Fatalf("%+v", r)
		}
	})
}

func TestNodeIndexResolvesManagers(t *testing.T) {
	c := detect.Certain
	facts := []detect.Fact{
		{Kind: "stack", Name: "node", Path: "package.json", Confidence: c}, {Kind: "package_manager", Name: "pnpm", Path: "package.json", Confidence: c},
		{Kind: "stack", Name: "node", Path: "packages/a/package.json", Confidence: c},
		{Kind: "stack", Name: "node", Path: "tools/b/package.json", Confidence: c}, {Kind: "lockfile", Name: "yarn", Path: "tools/b/yarn.lock", Confidence: c},
		{Kind: "stack", Name: "node", Path: "lone/package.json", Confidence: c},
	}
	idx := nodeIndex(facts)
	for dir, want := range map[string]string{".": "pnpm", "packages/a": "pnpm", "tools/b": "yarn", "lone": "pnpm"} {
		if idx[dir] != want {
			t.Errorf("%s: %q, want %q", dir, idx[dir], want)
		}
	}
	if got := nodeIndex(facts[2:3])["packages/a"]; got != "npm" {
		t.Errorf("no declaration anywhere defaults to npm, got %q", got)
	}
}

func TestLockDirsAndQuoting(t *testing.T) {
	for dir, want := range map[string]string{".": ".", "a": ". ..", "packages/a": ". .. ../..", "a/b/c": ". .. ../.. ../../.."} {
		if got := lockDirs(dir); got != want {
			t.Errorf("lockDirs(%q) = %q, want %q", dir, got, want)
		}
	}
	// every keyword is fixed text for grep -F inside single quotes
	for _, w := range append(append([]string{}, diagnose.InstallGuardWords...), diagnose.InstallCauseWords...) {
		if strings.ContainsAny(w, "'\\\n") || w == "" {
			t.Errorf("keyword %q cannot be quoted for sh", w)
		}
	}
}

func TestNodeGateKeepsTempAndCachesInTheInstallWorkspace(t *testing.T) {
	parent := t.TempDir()
	for pm, cache := range map[string]string{"npm": "npm_config_cache", "pnpm": "pnpm_config_store_dir", "bun": "BUN_INSTALL_CACHE_DIR", "yarn": "YARN_CACHE_FOLDER"} {
		t.Run(pm, func(t *testing.T) {
			h := newNodeHost(t)
			t.Setenv("TMPDIR", parent)
			h.write("package.json", manifest(pm+"@9.0.0", ""))
			h.write(lockfileOf[pm], "# lock\n")
			h.manager(pm, "9.0.0", `echo "TMP $TMPDIR"; echo "CACHE $`+cache+`"; exit 1`)
			r := h.run("node.test", ".")
			if !strings.Contains(r.out, "/.nself-ci/tmp\n") || !strings.Contains(r.out, "/.nself-ci/") || strings.Contains(r.out, "CACHE \n") {
				t.Fatalf("%+v", r)
			}
			if left, _ := os.ReadDir(parent); len(left) != 0 {
				t.Fatalf("the operator's TMPDIR gained %d entries", len(left))
			}
		})
	}
	t.Run("a check that installs nothing keeps its environment", func(t *testing.T) {
		h := newNodeHost(t)
		t.Setenv("TMPDIR", parent)
		h.write("package.json", manifest("npm@10.2.4", ""))
		h.write("node_modules/.keep", "")
		h.manager("npm", "10.2.4", `echo "TMP $TMPDIR"`)
		if r := h.run("node.test", "."); r.out != "TMP "+parent {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("pnpm never switches itself to the pinned version", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.write("node_modules/.keep", "")
		h.manager("pnpm", "9.1.0", `echo "SWITCH $pnpm_config_manage_package_manager_versions $npm_config_manage_package_manager_versions $pnpm_config_verify_deps_before_run"`)
		if r := h.run("node.test", "."); r.out != "SWITCH false false false" {
			t.Fatalf("%+v", r)
		}
	})
}

func TestYarnZeroInstallKeepsItsCommittedCache(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("yarn@4.1.0", ""))
	h.write("yarn.lock", "__metadata:\n  version: 8\n")
	h.write(".yarn/cache/.keep", "")
	h.manager("yarn", "4.1.0", `case "$1" in install) echo "FOLDER [$YARN_CACHE_FOLDER]"; exit 1;; esac
`)
	if r := h.run("node.test", "."); !strings.HasPrefix(r.out, "FOLDER []") {
		t.Fatalf("a committed .yarn/cache must stay the cache: %+v", r)
	}
	h2 := newNodeHost(t)
	h2.write("package.json", manifest("yarn@4.1.0", ""))
	h2.write("yarn.lock", "__metadata:\n  version: 8\n")
	h2.manager("yarn", "4.1.0", `case "$1" in install) echo "FOLDER [$YARN_CACHE_FOLDER]"; exit 1;; esac
`)
	if r := h2.run("node.test", "."); !strings.Contains(r.out, "/.nself-ci/yarn]") {
		t.Fatalf("%+v", r)
	}
}

func TestNodePrepareGuardAndCauseSurviveLongLines(t *testing.T) {
	for name, tc := range map[string]struct{ log, class string }{
		"yarn classic lifecycle with a long path": {`echo "error /` + strings.Repeat("deep/", 60) + `node_modules/x: Command failed."; echo "Exit code: 1"; echo "npm error code ENOTFOUND"`, "code"},
		"network cause after a long url":          {`echo "npm error code ENOTFOUND"; echo "npm error network request to https://registry.npmjs.org/` + strings.Repeat("a", 400) + ` failed, reason: getaddrinfo ENOTFOUND registry.npmjs.org"`, "infra"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newNodeHost(t)
			h.write("package.json", manifest("npm@10.2.4", ""))
			h.write("package-lock.json", "{}")
			h.manager("npm", "10.2.4", `i=0; while [ $i -lt 60 ]; do echo "npm info padding line number $i of this install log, which is long enough to exceed the short limit"; i=$((i+1)); done
`+tc.log+`
exit 1
`)
			r := h.run("node.test", ".")
			if r.exit != 97 || string(r.diag.Class) != tc.class {
				t.Fatalf("want %s: %+v", tc.class, r)
			}
		})
	}
}

func TestYarnPnPCacheOutlivesTheInstall(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("yarn@4.1.0", ""))
	h.write("yarn.lock", "__metadata:\n  version: 8\n")
	// Plug'n'Play: the installed tree is .pnp.cjs, which points at the archives in the cache
	h.manager("yarn", "4.1.0", `case "$1" in
install) mkdir -p "$YARN_CACHE_FOLDER"; touch "$YARN_CACHE_FOLDER/x.zip"; echo "$YARN_CACHE_FOLDER/x.zip" > .pnp.cjs;;
run) if [ -f "$(cat .pnp.cjs)" ]; then echo "CACHE-OK"; else echo "CACHE-GONE"; fi; echo "COREPACK ${COREPACK_HOME##*/}";;
esac
`)
	if r := h.run("node.test", "."); r.exit != 0 || r.out != "CACHE-OK\nCOREPACK corepack" {
		t.Fatalf("the yarn cache and the corepack home must survive for the check: %+v", r)
	}
}

func TestPnpmStoreOutlivesTheInstall(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	// pnpm's global virtual store: node_modules links into the store directory
	h.manager("pnpm", "9.1.0", `case "$1" in
install) mkdir -p "$npm_config_store_dir/pkg" node_modules; echo ok > "$npm_config_store_dir/pkg/index.js"; ln -s "$npm_config_store_dir/pkg" node_modules/pkg;;
run) if [ -f node_modules/pkg/index.js ]; then echo "STORE-OK"; else echo "STORE-GONE"; fi;;
esac
`)
	if r := h.run("node.test", "."); r.exit != 0 || r.out != "STORE-OK" {
		t.Fatalf("the pnpm store must survive for the check: %+v", r)
	}
}
