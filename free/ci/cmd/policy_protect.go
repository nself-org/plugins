package main

// Purpose: `nself ci policy protect [--repo o/r] [--branch b] [--dry-run]
// [--app-id n] | --hook | --audit <days> | --restore <snapshot>` (P7-CI-07,
// Epic D18, X18). Shows and applies branch protection that requires the
// `nself-ci` check (app-pinned), installs the pre-push hook, and lists
// default-branch commits without a green nself-ci. The work is in
// internal/protect; this file parses flags, resolves the repository and prints.
// Inputs:  argv after the key; env NSELF_CI_REPO (repository root), NSELF_CI_HOME
// (snapshots go to $NSELF_CI_HOME/protect/); gh authenticates itself.
// Outputs: text, or one v1 JSON envelope (--json); exit 0 ok, 1 failed (apply
// not verified, audit found commits, E642), 2 infrastructure, 3 precondition
// not met (E643: this plan has no branch protection; the hook and the auditor
// are the documented equivalent).
// Constraints: V15 only. Applying changes live protection, which is owner-named
// (P7-CI-55); --dry-run changes nothing. Only the added check differs from the
// live protection. A hook that exists is never overwritten.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/protect"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const protectUsage = "nself ci policy protect [--repo o/r] [--branch b] [--dry-run] [--app-id n] | --hook | --audit <days> | --restore <snapshot> [--json]"

type protectFlags struct {
	json, dryRun, hook bool
	repo, branch       string
	restore            string
	audit, appID       int
	auditSet           bool // --audit was given (so --audit 0 is an error, not "no audit")
}

func newProtectFlagSet(f *protectFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("policy protect", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&f.dryRun, "dry-run", false, "show the PUT body and change nothing")
	fs.BoolVar(&f.hook, "hook", false, "install the pre-push hook in this repository (never overwrites)")
	fs.StringVar(&f.repo, "repo", "", "GitHub repository, owner/name (default: the origin remote)")
	fs.StringVar(&f.branch, "branch", "", "branch (default: the repository's default branch)")
	fs.StringVar(&f.restore, "restore", "", "put this saved snapshot back")
	fs.IntVar(&f.audit, "audit", 0, "list default-branch commits of the last N days without a green nself-ci")
	fs.IntVar(&f.appID, "app-id", protect.AppID, "app id the required check is pinned to")
	return fs
}

func init() {
	registerV15("policy protect", subcommandEntry{V15Only: true, Handler: runPolicyProtect,
		Flags: func() *flag.FlagSet { return newProtectFlagSet(&protectFlags{}) }})
}

// protectEnv is everything the command touches outside its arguments.
type protectEnv struct {
	run  protect.Runner
	out  io.Writer
	root string
	home string
	now  func() time.Time
}

func runPolicyProtect(args []string) int {
	var f protectFlags
	fs := newProtectFlagSet(&f)
	pos, err := parseMixed(fs, args)
	fs.Visit(func(fl *flag.Flag) { f.auditSet = f.auditSet || fl.Name == "audit" })
	modes := 0
	for _, on := range []bool{f.hook, f.auditSet, f.restore != ""} {
		if on {
			modes++
		}
	}
	if err != nil || len(pos) != 0 || modes > 1 || (f.auditSet && f.audit <= 0) || f.appID <= 0 || (f.dryRun && (f.hook || f.auditSet)) {
		return usageErr(protectUsage)
	}
	root, err := evidenceRoot("")
	if err == nil {
		var home string
		if home, err = toolsHome(); err == nil {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			env := protectEnv{run: report.Capture, out: os.Stdout, root: root, home: home, now: time.Now}
			return env.dispatch(ctx, f)
		}
	}
	return exitForError(err)
}

func (e protectEnv) dispatch(ctx context.Context, f protectFlags) int {
	var err error
	switch {
	case f.hook: // needs the repository only to learn its default branch, and only without --branch
		err = e.installHook(ctx, f)
	case f.restore != "": // the snapshot names its repository
		err = e.restoreSnapshot(ctx, f)
	default:
		var repo string
		if repo, err = e.slug(ctx, f.repo); err != nil {
			break
		}
		if f.auditSet {
			return e.audit(ctx, f, repo)
		}
		err = e.protection(ctx, f, repo)
	}
	return protectExit(err, f.json)
}

// protectExit prints err and maps it to the exit code of the contract above.
func protectExit(err error, asJSON bool) int {
	if err == nil {
		return 0
	}
	var pe *protect.Error
	if errors.As(err, &pe) {
		code := 1
		if pe.Code == protect.CodeNoPlan {
			code = 3
		}
		if asJSON {
			_ = output.WriteError(os.Stdout, "ci policy protect", output.ErrorDetail{Code: pe.Code, Message: pe.Message, ExitCode: code}, nil)
		} else {
			fmt.Fprintln(os.Stderr, pe)
		}
		return code
	}
	return exitForError(err)
}

// slug resolves owner/name from --repo or the origin remote of the root.
func (e protectEnv) slug(ctx context.Context, given string) (string, error) {
	if given != "" {
		if strings.Count(given, "/") != 1 || strings.ContainsAny(given, " \t") {
			return "", fmt.Errorf("policy protect: --repo must look like owner/name, got %q", given)
		}
		return given, nil
	}
	url, exit, err := e.run(ctx, "git", "-C", e.root, "remote", "get-url", "origin")
	if err != nil || exit != 0 {
		return "", errors.New("policy protect: no --repo given and the origin remote cannot be read")
	}
	owner, name, ok := report.ParseRemote(url)
	if !ok {
		return "", fmt.Errorf("policy protect: origin is not a github.com remote; pass --repo owner/name")
	}
	return owner + "/" + name, nil
}

func (e protectEnv) branchOf(ctx context.Context, c protect.Client, f protectFlags, repo string) (string, error) {
	if f.branch != "" {
		return f.branch, nil
	}
	return c.DefaultBranch(ctx, repo)
}

type protectResult struct {
	Repo        string          `json:"repo"`
	Branch      string          `json:"branch"`
	DryRun      bool            `json:"dry_run"`
	Unprotected bool            `json:"unprotected"`
	AlreadySet  bool            `json:"already_required"`
	Added       []protect.Check `json:"added"`
	Snapshot    string          `json:"snapshot,omitempty"`
	Body        protect.Body    `json:"put_body"`
}

func (e protectEnv) protection(ctx context.Context, f protectFlags, repo string) error {
	c := protect.Client{Run: e.run}
	branch, err := e.branchOf(ctx, c, f, repo)
	if err != nil {
		return err
	}
	live, err := c.Protection(ctx, repo, branch)
	if err != nil {
		return e.noPlanHint(err)
	}
	plan, err := protect.Build(repo, branch, live, f.appID)
	if err != nil {
		return err
	}
	res := protectResult{Repo: repo, Branch: branch, DryRun: f.dryRun, Unprotected: plan.Unprotected, AlreadySet: plan.Noop(), Added: plan.Added, Body: plan.Body}
	if res.Added == nil {
		res.Added = []protect.Check{}
	}
	if !f.dryRun {
		if res.Snapshot, err = c.Apply(ctx, plan, live, filepath.Join(e.home, "protect"), e.now()); err != nil {
			return e.noPlanHint(err)
		}
	}
	return e.print(f.json, res, plan)
}

// noPlanHint adds the documented equivalent to an E643.
func (e protectEnv) noPlanHint(err error) error {
	var pe *protect.Error
	if errors.As(err, &pe) && pe.Code == protect.CodeNoPlan {
		pe.Message += "\nEquivalent without branch protection: `nself ci policy protect --hook` (pre-push hook) and `nself ci policy protect --audit 7` (lists commits without a green nself-ci)."
	}
	return err
}

func (e protectEnv) print(asJSON bool, res protectResult, plan protect.Plan) error {
	if asJSON {
		return output.WriteData(e.out, "ci policy protect", res, nil)
	}
	switch {
	case res.AlreadySet:
		fmt.Fprintf(e.out, "%s %s: nself-ci is already required; nothing to change\n", res.Repo, res.Branch)
	case res.DryRun:
		fmt.Fprintf(e.out, "%s %s: would add required check nself-ci (app %d)", res.Repo, res.Branch, res.Added[0].AppID)
		if res.Unprotected {
			fmt.Fprint(e.out, "; the branch is not protected, so this creates protection holding only that check")
		}
		fmt.Fprintf(e.out, "\nPUT body:\n%s", plan.JSON())
	default:
		fmt.Fprintf(e.out, "%s %s: nself-ci is now required (app %d); previous protection saved to %s\n", res.Repo, res.Branch, res.Added[0].AppID, res.Snapshot)
		if res.Unprotected {
			fmt.Fprintln(e.out, "the branch had no protection; --restore cannot put that back (removing protection is a manual step)")
		}
	}
	return nil
}

func (e protectEnv) restoreSnapshot(ctx context.Context, f protectFlags) error {
	snap, err := protect.LoadSnapshot(f.restore)
	if err != nil {
		return err
	}
	if f.repo != "" && f.repo != snap.Repo {
		return fmt.Errorf("policy protect: the snapshot is of %s, not %s", snap.Repo, f.repo)
	}
	if f.dryRun {
		body, err := protect.Normalise(snap.Protection)
		if err != nil {
			return err
		}
		_, err = e.out.Write(append([]byte(fmt.Sprintf("%s %s: would restore the snapshot of %s\n", snap.Repo, snap.Branch, snap.TakenAt)), protect.Plan{Body: body}.JSON()...))
		return err
	}
	before, err := (protect.Client{Run: e.run}).Restore(ctx, snap, filepath.Join(e.home, "protect"), e.now())
	if err != nil {
		return e.noPlanHint(err)
	}
	fmt.Fprintf(e.out, "%s %s: protection restored from %s; what it replaced is saved to %s\n", snap.Repo, snap.Branch, f.restore, before)
	return nil
}
