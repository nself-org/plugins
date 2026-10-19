package gates

// Purpose: P7-AUTO-16 Node preparation: the frozen install per declared
// manager, the packageManager pin, `astro sync`, the digest skip, and the
// failure classes of a failed install (code vs infrastructure).
// Constraints: fake package managers on a private PATH, POSIX only.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	for _, tool := range []string{"sh", "ps", "tail", "head", "cat", "mkdir"} {
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

func (h *nodeHost) run(id, workdir string) model.Check {
	h.t.Helper()
	d, ok := DefaultRegistry().Get(id)
	if !ok {
		h.t.Fatalf("unknown check %s", id)
	}
	r := Runner{Executor: &ciexec.Executor{}, Home: h.t.TempDir()}
	return r.Run(context.Background(), h.root, PlannedCheck{Def: d, Required: true, Workdir: workdir}, model.PipelineConfig{})
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

func TestNodePrepareFrozenPerManager(t *testing.T) {
	want := map[string]string{
		"npm":  "npm ci --prefer-offline",
		"pnpm": "pnpm install --frozen-lockfile --prefer-offline",
		"yarn": "yarn install --immutable",
		"bun":  "bun install --frozen-lockfile",
	}
	for pm, install := range want {
		t.Run(pm, func(t *testing.T) {
			h := newNodeHost(t)
			h.write("package.json", manifest(pm+"@9.0.0", ""))
			h.write(lockfileOf[pm], "__metadata:\nversion: 8\n") // a Yarn berry lockfile; harmless for the others
			h.manager(pm, "9.0.0", `echo "ARGS $*"; exit 1`)
			c := h.run("node.test", ".")
			if c.Result != "fail" || c.FailureClass != "code" {
				t.Fatalf("a failed install is a code failure: %+v", c)
			}
			if got := strings.TrimPrefix(strings.SplitN(c.Excerpt, "\n", 2)[0], "ARGS "); got != strings.TrimPrefix(install, pm+" ") {
				t.Fatalf("install argv %q, want %q (excerpt %q)", got, install, c.Excerpt)
			}
		})
	}
	t.Run("yarn classic", func(t *testing.T) {
		s := nodeSpec{Manager: "yarn"}
		if s.install() != "yarn install --frozen-lockfile --prefer-offline" {
			t.Fatal(s.install())
		}
	})
}

func TestNodePrepareRunsThenPasses(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", passing)
	c := h.run("node.test", ".")
	if c.Result != "pass" {
		t.Fatalf("%+v", c)
	}
	if strings.Contains(c.Excerpt, "INSTALL-NOISE") || !strings.Contains(c.Excerpt, "RUN test") {
		t.Fatalf("a passing install must not reach the check's output: %q", c.Excerpt)
	}
}

func TestNodePrepareLockfileOutOfSyncIsCode(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.manager("pnpm", "9.1.0", `echo ' ERR_PNPM_OUTDATED_LOCKFILE  Cannot install with "frozen-lockfile" because pnpm-lock.yaml is not up to date'; exit 1`)
	c := h.run("node.build", ".")
	if c.Result != "fail" || c.FailureClass != "code" || !strings.Contains(c.Excerpt, "ERR_PNPM_OUTDATED_LOCKFILE") || strings.Contains(c.Excerpt, prepareFailed) {
		t.Fatalf("%+v", c)
	}
}

func TestNodePrepareInfraFailuresPostError(t *testing.T) {
	for name, msg := range map[string]string{
		"network":  "npm ERR! code ETIMEDOUT\nnpm ERR! network request to https://registry.example/x failed\nnpm ERR! network This is a problem related to network connectivity.\nnpm ERR! network In most cases you are behind a proxy\nnpm ERR! A complete log of this run can be found in: /x/_logs/1.log",
		"disk":     "npm ERR! code ENOSPC\nnpm ERR! syscall write\nnpm ERR! errno -28\nnpm ERR! enospc ENOSPC: no space left on device, write",
		"dns":      "npm ERR! code EAI_AGAIN\nnpm ERR! request to https://registry.example failed, reason: getaddrinfo EAI_AGAIN registry.example",
		"filtered": "added 1 package\nwarn deprecated\nwarn deprecated\nwarn deprecated\nnpm ERR! code ECONNRESET\nnpm ERR! a\nnpm ERR! b\nnpm ERR! c\nnpm ERR! d",
	} {
		t.Run(name, func(t *testing.T) {
			h := newNodeHost(t)
			h.write("package.json", manifest("npm@10.2.4", ""))
			h.write("package-lock.json", "{}")
			h.manager("npm", "10.2.4", "cat <<'EOF'\n"+msg+"\nEOF\nexit 1\n")
			c := h.run("node.lint", ".")
			if c.Result != "error" || c.FailureClass != "infra" || !strings.HasPrefix(c.Excerpt, "E811:") {
				t.Fatalf("an infrastructure install failure must be error/infra with E811: %+v", c)
			}
		})
	}
}

func TestPackageManagerPin(t *testing.T) {
	cases := []struct {
		name, pin, found string
		bad              bool
	}{
		{"same", "9.1.0", "9.1.0", false}, {"newer minor", "9.1.0", "9.12.3", false}, {"older major", "9.1.0", "8.15.0", true},
		{"newer major", "4.1.0", "1.22.19", true}, {"no pin", "", "8.0.0", false}, {"unreadable", "9.1.0", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if why := (nodeSpec{Manager: "pnpm", Pin: tc.pin}).pinMismatch(tc.found); (why != "") != tc.bad {
				t.Fatalf("pin %q found %q: %q", tc.pin, tc.found, why)
			}
		})
	}
	t.Run("a mismatch posts error", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0+sha512.abc", ""))
		h.manager("pnpm", "8.15.0", passing)
		c := h.run("node.test", ".")
		if c.Result != "error" || c.FailureClass != "infra" || !strings.Contains(c.Excerpt, "E811") || !strings.Contains(c.Excerpt, "pnpm@9.1.0") {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("a satisfied pin runs", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.manager("pnpm", "9.12.3", passing)
		if c := h.run("node.test", "."); c.Result != "pass" {
			t.Fatalf("%+v", c)
		}
	})
}

func TestMissingManagerE614(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	c := h.run("node.test", ".")
	if c.Result != "error" || c.FailureClass != "infra" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Install the declared Node package manager") {
		t.Fatalf("a missing manager is E614 with the catalogue remediation, never a failure: %+v", c)
	}
}

func TestAstroSyncWhenDeclared(t *testing.T) {
	for pm, sync := range map[string]string{"npm": "npm exec --no-install -- astro sync", "pnpm": "pnpm exec astro sync", "yarn": "yarn run astro sync", "bun": "bun run astro sync"} {
		s := nodeSpec{Manager: pm, Astro: true, HasModules: true}
		for _, id := range []string{"node.typecheck", "node.build"} {
			if got := strings.Join(s.wrap(id, []string{pm, "run", "x"}), " "); !strings.Contains(got, sync+" && "+pm+" run x") {
				t.Fatalf("%s %s: %s", pm, id, got)
			}
		}
		for _, id := range []string{"node.lint", "node.test"} {
			if got := strings.Join(s.wrap(id, []string{pm, "run", "x"}), " "); strings.Contains(got, "astro") {
				t.Fatalf("%s %s must not sync: %s", pm, id, got)
			}
		}
	}
	t.Run("runs before the check", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", `"devDependencies":{"astro":"^5.0.0"},`))
		h.write("node_modules/.keep", "") // installed already (no lockfile to install from)
		h.manager("pnpm", "9.1.0", passing)
		c := h.run("node.typecheck", ".")
		if c.Result != "pass" || c.Excerpt != "EXEC exec astro sync\nRUN typecheck" {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("not declared, not run", func(t *testing.T) {
		h := newNodeHost(t)
		h.write("package.json", manifest("pnpm@9.1.0", ""))
		h.manager("pnpm", "9.1.0", passing)
		if c := h.run("node.typecheck", "."); c.Excerpt != "RUN typecheck" {
			t.Fatalf("%+v", c)
		}
	})
}

func TestNodePrepareSkipsWhenDigestMatches(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", passing)
	spec := nodeSpecFor(h.root, PlannedCheck{Workdir: "."})
	if spec.Prepared || spec.Digest == "" {
		t.Fatalf("a fresh checkout is not prepared: %+v", spec)
	}
	h.write(nodePreparedMarker, spec.Digest)
	spec = nodeSpecFor(h.root, PlannedCheck{Workdir: "."})
	if !spec.Prepared {
		t.Fatalf("a matching marker skips the install: %+v", spec)
	}
	if got := strings.Join(spec.wrap("node.test", []string{"npm", "run", "test"}), " "); got != "npm run test" {
		t.Fatalf("nothing to prepare, got %q", got)
	}
	h.write("package-lock.json", `{"changed":true}`)
	if nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Prepared {
		t.Fatal("a changed lockfile must install again")
	}
}

func TestNodePrepareWorkspaceMemberInstallsAtTheLockfile(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", `{"private":true}`)
	h.write("pnpm-lock.yaml", "lockfileVersion: 9\n")
	h.write("packages/a/package.json", manifest("", ""))
	s := nodeSpecFor(h.root, PlannedCheck{Workdir: "packages/a"})
	if s.Manager != "pnpm" || s.LockDir != "." || s.Rel != "../.." {
		t.Fatalf("%+v", s)
	}
	if got := strings.Join(s.wrap("node.test", []string{"pnpm", "run", "test"}), " "); !strings.Contains(got, "cd ../.. && pnpm install --frozen-lockfile") {
		t.Fatal(got)
	}
	for work, rel := range map[string]string{"a": "..", "a/b/c": "../../.."} {
		if got := relUp(work, "."); got != rel {
			t.Fatalf("relUp(%s) = %s", work, got)
		}
	}
	if relUp("p/a/b", "p") != "../.." || relUp("p", "p") != "." {
		t.Fatal("relUp")
	}
}

func TestNodeWithoutLockfileKeepsEarlierBehaviour(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.manager("npm", "10.2.4", passing)
	if c := h.run("node.test", "."); c.Result != "pass" || c.Excerpt != "RUN test" {
		t.Fatalf("%+v", c)
	}
}

func TestNodePrepareInstallWithoutNodeModules(t *testing.T) {
	// Yarn PnP and a project without dependencies install no node_modules
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", `case "$1" in ci) echo "up to date";; run) echo "RUN $2";; esac
`)
	if c := h.run("node.test", "."); c.Result != "pass" || c.Excerpt != "RUN test" {
		t.Fatalf("%+v", c)
	}
}

func TestNodePrepareInstallFailureClasses(t *testing.T) {
	infra := map[string]string{
		"npm enotfound":       "npm error code ENOTFOUND\nnpm error syscall getaddrinfo\nnpm error errno ENOTFOUND\nnpm error network request to https://registry.npmjs.org/x failed, reason: getaddrinfo ENOTFOUND registry.npmjs.org\nnpm error A complete log of this run can be found in: /x/1.log",
		"socket hang up":      "npm warn deprecated a\nnpm error network socket hang up\nnpm error network This is a problem related to network connectivity.\nnpm error network In most cases you are behind a proxy or have bad network settings.\nnpm error log",
		"pnpm meta fetch":     " ERR_PNPM_META_FETCH_FAIL  GET https://registry.npmjs.org/x: Client network socket disconnected\nThis error happened while installing the dependencies of x",
		"yarn network":        "info There appears to be trouble with your network connection. Retrying...\nerror An unexpected error occurred: \"https://registry.yarnpkg.com/x: getaddrinfo ENOTFOUND registry.yarnpkg.com\".",
		"registry refuses":    "npm error code ECONNREFUSED\nnpm error syscall connect\nnpm error FetchError: request to https://registry.npmjs.org/x failed, reason: connect ECONNREFUSED 10.1.2.3:443\nnpm error log",
		"pnpm 503":            " ERR_PNPM_FETCH_503  GET https://registry.npmjs.org/x: Service Unavailable - 503",
		"disk":                "npm error code ENOSPC\nnpm error nospc",
		"transient then real": "npm warn ECONNRESET retrying\nnpm warn ECONNRESET retrying\nnpm error code ENOSPC\nnpm error log",
	}
	code := map[string]string{
		"out of sync":          "npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.\nnpm error Missing: x@1.0.0 from lock file",
		"sync after transient": "npm warn ECONNRESET retrying\nnpm warn ECONNRESET retrying\nERR_PNPM_OUTDATED_LOCKFILE  Cannot install with \"frozen-lockfile\" because pnpm-lock.yaml is not up to date",
		"berry sync":           "YN0028: The lockfile would have been modified by this install, which is explicitly forbidden.",
		"classic sync":         "error Your lockfile needs to be updated, but yarn was run with `--frozen-lockfile`.",
		"bun sync":             "error: lockfile had changes, but lockfile is frozen",
		"not found package":    " ERR_PNPM_FETCH_404  GET https://registry.npmjs.org/nope: Not Found - 404",
		"npm 404":              "npm error code E404\nnpm error 404 Not Found - GET https://registry.npmjs.org/nope - Not found",
		"postinstall test":     "> app@1.0.0 postinstall\n--- FAIL: TestRetry (ETIMEDOUT expected)\nFAIL\nnpm error code 1",
		"postinstall tool":     "sh: 1: tsc: not found\nnpm error code 127",
		"postinstall db":       "> app@1.0.0 postinstall\n> node scripts/migrate.js\nError: connect ECONNREFUSED 127.0.0.1:5432\nnpm error command failed",
		"node assertion":       "AssertionError [ERR_ASSERTION]: expected ECONNREFUSED to be handled\nnpm error code 1",
	}
	for want, set := range map[string]map[string]string{"infra": infra, "code": code} {
		for name, msg := range set {
			t.Run(want+"/"+name, func(t *testing.T) {
				h := newNodeHost(t)
				h.write("package.json", manifest("npm@10.2.4", ""))
				h.write("package-lock.json", "{}")
				h.manager("npm", "10.2.4", "cat <<'EOF'\n"+msg+"\nEOF\nexit 1\n")
				c := h.run("node.lint", ".")
				switch want {
				case "infra":
					if c.Result != "error" || c.FailureClass != "infra" || !strings.HasPrefix(c.Excerpt, "E811:") {
						t.Fatalf("%+v", c)
					}
				default:
					if c.Result != "fail" || c.FailureClass != "code" || strings.Contains(c.Excerpt, "E811") {
						t.Fatalf("%+v", c)
					}
				}
			})
		}
	}
}

func TestNodePrepareLongLogKeepsBothEnds(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.manager("npm", "10.2.4", `echo "npm error npm ci can only install packages when your package.json and package-lock.json are in sync"
i=0; while [ $i -lt 120 ]; do echo "npm error Missing: package-$i@1.0.0 from lock file"; i=$((i+1)); done
echo "npm error A complete log of this run can be found in: /x/final.log"
exit 1
`)
	c := h.run("node.build", ".")
	if c.Result != "fail" || c.FailureClass != "code" || !strings.Contains(c.Excerpt, "can only install packages") || !strings.Contains(c.Excerpt, "/x/final.log") || len(c.Excerpt) > 1100 {
		t.Fatalf("%d bytes: %+v", len(c.Excerpt), c)
	}
}

func TestPackageManagerPinIsProbedInTheWorkdir(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", ""))
	h.write("sub/package.json", manifest("pnpm@10.0.0", ""))
	h.write("sub/pin-version", "10.0.0")
	h.manager("pnpm", "$(cat pin-version 2>/dev/null || echo 9.1.0)", passing)
	if c := h.run("node.test", "sub"); c.Result != "pass" {
		t.Fatalf("a member pinned to another major must be probed where it runs: %+v", c)
	}
	if c := h.run("node.test", "."); c.Result != "pass" {
		t.Fatalf("%+v", c)
	}
}

func TestAstroSyncNeedsDependencies(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("pnpm@9.1.0", `"devDependencies":{"astro":"^5.0.0"},`))
	h.manager("pnpm", "9.1.0", passing)
	if c := h.run("node.build", "."); c.Excerpt != "RUN build" {
		t.Fatalf("no lockfile and no node_modules: nothing to sync against, earlier behaviour: %+v", c)
	}
	h.write("node_modules/.keep", "")
	if c := h.run("node.build", "."); c.Excerpt != "EXEC exec astro sync\nRUN build" {
		t.Fatalf("%+v", c)
	}
}

func TestNodeDigestFollowsTheManifest(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	a := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest
	h.write("package.json", manifest("npm@10.2.4", `"dependencies":{"left-pad":"1.0.0"},`))
	if b := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest; a == b || a == "" {
		t.Fatal("a manifest edit with an unchanged lockfile must install again")
	}
}

func TestShellJoinQuotes(t *testing.T) {
	if got := shellJoin([]string{"npm", "run", "a b", "it's", "--", "--coverage"}); got != `npm run 'a b' 'it'\''s' -- --coverage` {
		t.Fatal(got)
	}
}

func TestNodeDigestFollowsWorkspaceMembers(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", `{"private":true,"workspaces":["packages/*"]}`)
	h.write("package-lock.json", "{}")
	h.write("packages/a/package.json", manifest("", ""))
	h.write("node_modules/dep/package.json", `{"name":"dep"}`)
	a := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest
	h.write("node_modules/dep/package.json", `{"name":"dep2"}`)
	if b := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest; a != b {
		t.Fatal("node_modules is not part of the digest")
	}
	h.write("packages/a/package.json", manifest("", `"dependencies":{"left-pad":"1.0.0"},`))
	if b := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest; a == b {
		t.Fatal("a member's manifest edit with an unchanged lockfile must install again")
	}
}

func TestNodePrepareGuardsReadTheMiddleOfALongLog(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	// the assertion sits far from both ends of a 6 KB log, then a transient reset ends it
	h.manager("npm", "10.2.4", `i=0; while [ $i -lt 40 ]; do echo "npm info padding line number $i of the head part of this install log"; i=$((i+1)); done
echo "AssertionError [ERR_ASSERTION]: postinstall self-check"
i=0; while [ $i -lt 60 ]; do echo "npm info padding line number $i of the tail part of this install log"; i=$((i+1)); done
echo "npm error code ECONNRESET"
exit 1
`)
	c := h.run("node.test", ".")
	if c.Result != "fail" || c.FailureClass != "code" || strings.Contains(c.Excerpt, "E811") {
		t.Fatalf("%+v", c)
	}
}

func TestNodeDigestPrefersTheShrinkwrap(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	h.write("npm-shrinkwrap.json", "{}")
	a := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest
	h.write("npm-shrinkwrap.json", `{"changed":1}`)
	if b := nodeSpecFor(h.root, PlannedCheck{Workdir: "."}).Digest; a == b {
		t.Fatal("npm installs from the shrinkwrap: editing it must change the digest")
	}
}

func TestNodePrepareHugeLogIsCodeNotInfra(t *testing.T) {
	h := newNodeHost(t)
	h.write("package.json", manifest("npm@10.2.4", ""))
	h.write("package-lock.json", "{}")
	// 350 KB; the assertion is outside both retained ends; the log ends with a reset
	h.manager("npm", "10.2.4", `i=0; while [ $i -lt 2200 ]; do echo "npm info padding line number $i of the head part of this very long install log, 80 bytes."; i=$((i+1)); done
echo "AssertionError [ERR_ASSERTION]: postinstall self-check"
i=0; while [ $i -lt 2200 ]; do echo "npm info padding line number $i of the tail part of this very long install log, 80 bytes."; i=$((i+1)); done
echo "npm error code ECONNRESET"
exit 1
`)
	c := h.run("node.test", ".")
	if c.Result != "fail" || c.FailureClass != "code" || strings.Contains(c.Excerpt, "E811") {
		t.Fatalf("%+v", c)
	}
}
