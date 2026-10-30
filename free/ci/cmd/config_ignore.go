package main

// Purpose: `nself ci config ignore add <fingerprint> --reason <text> [--by <who>]`
// (P7-AUTO-17): appends a reviewed entry to the repository's .gitleaksignore so
// the secrets.gitleaks review rule (internal/gates/gitleaks_ignore.go) passes.
// Inputs:  one gitleaks fingerprint (commit:path:rule:line or path:rule:line),
// --reason, optional --by and --repo (env NSELF_CI_REPO; default: the
// current directory).
// Outputs: one line of text; exit 0 (added, or already present), 1 (refused
// or not written), 2 (usage).
// Constraints: V15 only; side-effect class write (one local file, nothing
// else). The fingerprint is never printed: not on success, not on refusal and
// not in a flag error. An argument with a known token shape (internal/redact)
// or a high-entropy segment is refused as a possible real secret.

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const ignoreAddCommand = "ci config ignore add"

// ignoreDeps are the process facts `config ignore add` reads.
type ignoreDeps struct {
	Getenv func(string) string
	Getwd  func() (string, error)
	Now    func() time.Time
}

type ignoreAddOpts struct {
	reason, by, repo string
}

func newIgnoreAddFlags() (*flag.FlagSet, *ignoreAddOpts) {
	o := &ignoreAddOpts{}
	fs := flag.NewFlagSet(ignoreAddCommand, flag.ContinueOnError)
	fs.StringVar(&o.reason, "reason", "", "why the finding is not a real secret (required)")
	fs.StringVar(&o.by, "by", "", "who reviewed the finding")
	fs.StringVar(&o.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	return fs, o
}

func ignoreAddFlags() *flag.FlagSet { fs, _ := newIgnoreAddFlags(); return fs }

func init() {
	registerV15("config ignore add", subcommandEntry{
		V15Only: true,
		Flags:   ignoreAddFlags,
		Handler: func(args []string) int {
			return runIgnoreAdd(args, os.Stdout, os.Stderr, ignoreDeps{Getenv: os.Getenv, Getwd: os.Getwd, Now: time.Now})
		},
	})
}

// parseIgnoreAdd parses flags before and after the positional fingerprint.
// Flag errors are reported without the offending token, which may be the value.
func parseIgnoreAdd(args []string) (*ignoreAddOpts, []string, error) {
	fs, o := newIgnoreAddFlags()
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, nil, errors.New("usage: nself ci config ignore add <fingerprint> --reason <text> [--by <who>] [--repo <dir>]")
		}
		args = fs.Args()
		if len(args) == 0 {
			return o, pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

func runIgnoreAdd(args []string, stdout, stderr io.Writer, d ignoreDeps) int {
	o, pos, err := parseIgnoreAdd(args)
	if err != nil {
		return ignoreFail(stderr, "E812", err.Error(), 2)
	}
	if len(pos) != 1 {
		return ignoreFail(stderr, "E812", "exactly one fingerprint is required", 2)
	}
	if strings.TrimSpace(o.reason) == "" {
		return ignoreFail(stderr, "E812", "--reason is required", 2)
	}
	fp := pos[0]
	if looksSecret(fp) || highEntropy(fp) || looksSecret(o.reason) || looksSecret(o.by) {
		return ignoreFail(stderr, "E812", "refused: an argument looks like a real secret, not a gitleaks fingerprint; nothing was written and the value is not shown", 1)
	}
	valid, withCommit := gates.FingerprintCommit(fp)
	if !valid {
		return ignoreFail(stderr, "E812", "the fingerprint is not in a gitleaks form (commit:path:rule:line or path:rule:line)", 1)
	}
	root := o.repo
	if root == "" {
		root = d.Getenv("NSELF_CI_REPO")
	}
	if root == "" {
		root, err = d.Getwd()
	} else {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return ignoreFail(stderr, "E812", "the repository root cannot be resolved", 2)
	}
	added, err := gates.AddReviewedIgnore(root, fp, o.reason, o.by, d.Now())
	if err != nil {
		return ignoreFail(stderr, "E812", strings.TrimPrefix(err.Error(), "E812: "), 1)
	}
	form := "no-git"
	if withCommit {
		form = "git"
	}
	if added {
		_, _ = fmt.Fprintf(stdout, "added one reviewed %s entry (%s form)\n", gates.IgnoreFile, form)
	} else {
		_, _ = fmt.Fprintf(stdout, "the entry is already in %s; nothing changed\n", gates.IgnoreFile)
	}
	return 0
}

func ignoreFail(stderr io.Writer, code, msg string, exit int) int {
	_, _ = fmt.Fprintln(stderr, code+": "+msg)
	return exit
}

// looksSecret reports whether s holds a token shape the redactor masks.
func looksSecret(s string) bool {
	if s == "" {
		return false
	}
	f := redact.New().NewFilter()
	out := append(f.Redact([]byte(s)), f.Flush()...)
	return !bytes.Equal(out, []byte(s))
}

// highEntropy reports whether a fingerprint segment (split on ':', '/', '\'
// and '.') of 20 or more bytes mixes letters and digits at 3.5 bits per byte
// or more. A leading commit hash is skipped, since hex is what a commit is.
func highEntropy(fp string) bool {
	if valid, withCommit := gates.FingerprintCommit(fp); valid && withCommit {
		fp = fp[strings.Index(fp, ":")+1:]
	}
	segs := strings.FieldsFunc(fp, func(r rune) bool { return r == ':' || r == '/' || r == '\\' || r == '.' })
	for _, s := range segs {
		if len(s) >= 20 && strings.ContainsAny(s, "0123456789") && strings.ContainsAny(strings.ToLower(s), "abcdefghijklmnopqrstuvwxyz") && entropy(s) >= 3.5 {
			return true
		}
	}
	return false
}

// entropy is the Shannon entropy of s in bits per byte.
func entropy(s string) float64 {
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	h, n := 0.0, float64(len(s))
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / n
			h -= p * math.Log2(p)
		}
	}
	return h
}
