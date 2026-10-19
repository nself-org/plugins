package gates

// Purpose: gate preparation for the Node checks (P7-AUTO-16, cap:ci.gate-prep):
// a frozen install per declared package manager, `astro sync` before the type
// check and build of an Astro project, and the packageManager version pin.
// Inputs:  detector facts (the manager of a directory, its lockfiles) at plan
// time; package.json and the lockfile are read by the script when the job runs.
// Outputs: the argv of a planned node check (PlannedCheck.Argv and .Tool, as the
// Python checks do): `sh -c` of one script that prepares, then execs the check.
// Constraints: every executor attempt runs in its own workspace copy, so the
// prepare step and the check share ONE invocation; nothing installs globally and
// the manager caches live inside the workspace (removed with it); an install runs
// only when neither the working directory nor the lockfile directory has a
// node_modules (a developer's installed tree is used as it is, and a copied
// pnpm tree must never be reinstalled over without a TTY); a project without a
// lockfile keeps the earlier behaviour (no install); the install log is shown
// only when the install fails, as head, keyword lines and tail, followed by a
// sentinel that diagnose.Classify turns into code or infrastructure; POSIX
// shells only (Windows is out of scope, D-1029).

import (
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/diagnose"
)

// nodeDirs maps each Node project directory to its package manager.
type nodeDirs map[string]string

// nodeIndex finds the package manager of every Node directory from the facts:
// its own packageManager or lockfile, else the nearest ancestor's, else npm.
func nodeIndex(facts []detect.Fact) nodeDirs {
	declared, locked, dirs := map[string]string{}, map[string][]string{}, nodeDirs{}
	for _, f := range facts {
		if f.Confidence != detect.Certain {
			continue
		}
		switch {
		case f.Kind == "stack" && f.Name == "node":
			dirs[path.Dir(f.Path)] = ""
		case f.Kind == "package_manager" && nodeLockfiles[f.Name] != nil:
			declared[path.Dir(f.Path)] = f.Name
		case f.Kind == "lockfile" && nodeLockfiles[f.Name] != nil:
			locked[path.Dir(f.Path)] = append(locked[path.Dir(f.Path)], f.Name)
		}
	}
	own := func(d string) string {
		if m := declared[d]; m != "" {
			return m
		}
		for _, pm := range nodeLockOrder {
			for _, l := range locked[d] {
				if l == pm {
					return pm
				}
			}
		}
		return ""
	}
	for d := range dirs {
		for a := d; ; a = path.Dir(a) {
			if m := own(a); m != "" {
				dirs[d] = m
				break
			}
			if a == "." || a == "/" || a == "" {
				dirs[d] = "npm"
				break
			}
		}
	}
	return dirs
}

// mod sets the tool and argv of one planned node check.
func (n nodeDirs) mod(dir, id string) func(*PlannedCheck) {
	return func(p *PlannedCheck) {
		pm := n[dir]
		if pm == "" {
			pm = "npm"
		}
		p.Tool, p.Argv = pm, nodeArgv(pm, id, dir)
	}
}

// nodeArgv is the argv of one node check for the manager: the prepare script, or
// the plain `<pm> run <script>` where no shell script can run.
func nodeArgv(pm, id, dir string) []string {
	run := []string{pm, "run", strings.TrimPrefix(id, "node.")}
	if id == "node.coverage" {
		run = []string{pm, "run", "test", "--", "--coverage"}
	}
	if runtime.GOOS == "windows" {
		return run
	}
	return []string{"sh", "-c", nodeScript(pm, id, strings.Join(run, " "), dir)}
}

// nodeScript is the shell text: caches, the frozen install when needed, astro
// sync when needed, then the check.
func nodeScript(pm, id, run, dir string) string {
	var b strings.Builder
	b.WriteString("LOCK=; LF=\nfor d in " + lockDirs(dir) + "; do\n  for f in " + strings.Join(nodeLockfiles[pm], " ") + "; do\n")
	b.WriteString("    if [ -f \"$d/$f\" ]; then LOCK=$d; LF=$f; break 2; fi\n  done\ndone\n")
	b.WriteString(managerEnv(pm))
	b.WriteString("if [ -n \"$LOCK\" ] && [ ! -d node_modules ] && [ ! -d \"$LOCK/node_modules\" ]; then\n")
	b.WriteString(pinCheck(pm))
	b.WriteString("  NSELF_CI_DIR=\"$PWD/.nself-ci\"; mkdir -p \"$NSELF_CI_DIR/tmp\"; LOG=\"$NSELF_CI_DIR/install.log\"\n")
	b.WriteString("  ( cd \"$LOCK\" && " + installEnv(pm) + "; " + installCmd(pm) + " ) > \"$LOG\" 2>&1 || {\n")
	b.WriteString(failureReport())
	b.WriteString("    echo " + diagnose.PrepareFailed + "\n    exit " + strconv.Itoa(diagnose.PrepareExit) + "\n  }\n  rm -rf \"$NSELF_CI_DIR\"\nfi\n")
	if id == "node.typecheck" || id == "node.build" {
		b.WriteString("if { [ -d node_modules ] || [ -d \"${LOCK:-.}/node_modules\" ] || [ -f \"${LOCK:-.}/.pnp.cjs\" ]; } && grep -q '\"astro\"[[:space:]]*:' package.json 2>/dev/null; then\n")
		b.WriteString("  " + astroSync(pm) + " || exit $?\nfi\n")
	}
	b.WriteString("exec " + run + "\n")
	return b.String()
}

// lockDirs lists the directories a lockfile may sit in: the working directory
// and each ancestor up to the workspace root (a member installs at the root).
func lockDirs(dir string) string {
	out := []string{"."}
	for d := path.Clean(dir); d != "." && d != "/" && d != ""; d = path.Dir(d) {
		out = append(out, strings.TrimPrefix(strings.Repeat("/..", len(out)), "/"))
	}
	return strings.Join(out, " ")
}

// installCmd is the frozen install of the manager, run in the lockfile directory
// ($LF is the lockfile found there).
func installCmd(pm string) string {
	switch pm {
	case "npm":
		return "npm ci --prefer-offline"
	case "pnpm":
		return "pnpm install --frozen-lockfile --prefer-offline"
	case "yarn":
		return `if grep -q __metadata: "$LF"; then yarn install --immutable; else yarn install --frozen-lockfile --prefer-offline; fi`
	}
	return "bun install --frozen-lockfile"
}

// astroSync is the manager's way to run the project-local astro binary.
func astroSync(pm string) string {
	switch pm {
	case "npm":
		return "npm exec --no-install -- astro sync"
	case "pnpm":
		return "pnpm exec astro sync"
	case "yarn":
		return "yarn run astro sync"
	}
	return "bun run astro sync"
}

// managerEnv is set for every check of the manager: pnpm must not switch itself to
// the pinned version (it downloads it into a global store, so the pin check below
// and an offline host would both be defeated), nor reinstall stale dependencies
// before every `pnpm run` (pnpm 11+; the gate installs, or uses the tree as it is).
func managerEnv(pm string) string {
	if pm == "pnpm" {
		return "export pnpm_config_manage_package_manager_versions=false npm_config_manage_package_manager_versions=false pnpm_config_verify_deps_before_run=false npm_config_verify_deps_before_run=false\n"
	}
	return ""
}

// installEnv keeps the install's cache and temp directory inside the workspace
// (removed with it, so a killed install leaves nothing in the operator's TMPDIR),
// only the manager's own variables: npm warns about another manager's. A
// committed Yarn .yarn/cache (zero-install) stays the cache.
func installEnv(pm string) string {
	env := `export TMPDIR="$NSELF_CI_DIR/tmp" COREPACK_HOME="$NSELF_CI_DIR/corepack"; `
	switch pm {
	case "npm":
		return env + `export npm_config_cache="$NSELF_CI_DIR/npm"`
	case "pnpm":
		return env + `export pnpm_config_store_dir="$NSELF_CI_DIR/pnpm" npm_config_store_dir="$NSELF_CI_DIR/pnpm" COREPACK_ENABLE_DOWNLOAD_PROMPT=0`
	case "yarn":
		return env + `[ -d .yarn/cache ] || export YARN_CACHE_FOLDER="$NSELF_CI_DIR/yarn"; export YARN_ENABLE_GLOBAL_CACHE=false COREPACK_ENABLE_DOWNLOAD_PROMPT=0`
	}
	return env + `export BUN_INSTALL_CACHE_DIR="$NSELF_CI_DIR/bun"`
}

// pinCheck refuses an install when package.json pins another MAJOR version of
// the manager than the one on PATH (lockfile formats and the frozen flags differ
// across majors). It runs only where an install would; a pin that cannot be read
// is no pin.
func pinCheck(pm string) string {
	return `  PIN=; for pj in package.json "$LOCK/package.json"; do
    PIN=$(sed -n 's/.*"packageManager"[[:space:]]*:[[:space:]]*"` + pm + `@\([0-9][0-9]*\).*/\1/p' "$pj" 2>/dev/null | head -n 1)
    [ -n "$PIN" ] && break
  done
  if [ -n "$PIN" ]; then
    HAVE=$(COREPACK_ENABLE_NETWORK=0 COREPACK_ENABLE_DOWNLOAD_PROMPT=0 ` + pm + ` --version 2>/dev/null | sed -n 's/^[^0-9]*\([0-9][0-9]*\)\..*/\1/p' | head -n 1)
    if [ -n "$HAVE" ] && [ "$HAVE" != "$PIN" ]; then
      echo "E811: package.json pins ` + pm + ` $PIN.x but ` + pm + ` $HAVE is installed; install the pinned major (corepack enable) and retry"
      echo ` + diagnose.PinMismatch + `
      exit ` + strconv.Itoa(diagnose.PinExit) + `
    fi
  fi
`
}

// failureReport prints what the classifier needs of a failed install, within the
// 8 KiB tail a run keeps: the whole log when short, else its head, the first 12
// lines that carry a guard and the last 12 that carry a cause (warning lines
// excluded from both, as the classifier drops them), each as a 200 byte window
// that starts before the first keyword (never cut at the front, so the keyword and
// the code after it survive a long path), and the log's tail.
func failureReport() string {
	window := func(list []string) string {
		return `awk -v w='` + strings.Join(list, "|") + `' 'BEGIN{n=split(w,k,"|")} {l=tolower($0); if(l ~ /(^|[^a-z0-9])warn([^a-z0-9]|$)/) next; for(i=1;i<=n;i++){p=index(l,k[i]); if(p){s=p>60?p-59:1; print substr($0,s,200); next}}}' "$LOG"`
	}
	return `    n=$(( $(wc -c < "$LOG") + 0 ))
    if [ "$n" -le 3500 ]; then cat "$LOG"; else
      head -c 800 "$LOG"; echo
      echo ` + diagnose.InstallGuardsMark + `
      ` + window(diagnose.InstallGuardWords) + ` | head -n 12
      echo ` + diagnose.InstallCausesMark + `
      ` + window(diagnose.InstallCauseWords) + ` | tail -n 12
      echo ` + diagnose.InstallTailMark + `
      tail -c 1000 "$LOG"
    fi
`
}
