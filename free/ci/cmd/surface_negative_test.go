package main

// Purpose: proves the surface test can fail (P7-CI-62, CI D31 review): each fixture breaks one rule of
//          commands.yaml or surfaceClasses and must produce a problem naming the path.
// Constraints: pure; the spec is deep-copied through YAML before each mutation.

import (
	"flag"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func cloneSpec(t *testing.T, s specFile) specFile {
	t.Helper()
	raw, err := yaml.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var c specFile
	if err = yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func rowOf(s *specFile, path string) *specRow {
	for i := range s.Commands {
		if s.Commands[i].Path == path {
			return &s.Commands[i]
		}
	}
	panic("no row " + path)
}

func flagOf(r *specRow, name string) *specFlag {
	for i := range r.Flags {
		if r.Flags[i].Name == name {
			return &r.Flags[i]
		}
	}
	panic("no flag " + name)
}

func TestSpecProblemsNegativeFixtures(t *testing.T) {
	base := loadSpec(t)
	if p := specProblems(base); len(p) != 0 {
		t.Fatalf("the real spec has problems: %v", p)
	}
	cases := []struct {
		name, want string
		mutate     func(*specFile)
	}{
		{"missing key", "nodes show: table key has no spec row", func(s *specFile) {
			for i, r := range s.Commands {
				if r.Path == "nodes show" {
					s.Commands = append(s.Commands[:i], s.Commands[i+1:]...)
					return
				}
			}
		}},
		{"extra row", "bogus: spec row has no table key", func(s *specFile) { s.Commands = append(s.Commands, specRow{Path: "bogus", SideEffect: "read"}) }},
		{"undefined flag", "nodes list: spec flag --nope is not defined", func(s *specFile) {
			r := rowOf(s, "nodes list")
			r.Flags = append(r.Flags, specFlag{Name: "nope", Type: "bool"})
		}},
		{"artifacts understated", "artifacts: side_effect read is lower than destructive", func(s *specFile) { rowOf(s, "artifacts").SideEffect = "read" }},
		{"resolve understated", "runs resolve: side_effect write is lower than destructive", func(s *specFile) { rowOf(s, "runs resolve").SideEffect = "write" }},
		{"revoke understated", "nodes revoke: side_effect remote is lower than destructive", func(s *specFile) { rowOf(s, "nodes revoke").SideEffect = "remote" }},
		{"label understated", "nodes label: side_effect write is lower than remote", func(s *specFile) { rowOf(s, "nodes label").SideEffect = "write" }},
		{"doctor alert exposed", "doctor: flag --alert reaches a remote action", func(s *specFile) { flagOf(rowOf(s, "doctor"), "alert").CLIOnly = false }},
		{"sync cmd exposed", "nodes sync: flag --cmd reaches a remote action", func(s *specFile) { flagOf(rowOf(s, "nodes sync"), "cmd").CLIOnly = false }},
		{"confirm dropped", "nodes projects remove: remote row is exposed without a confirm block", func(s *specFile) { rowOf(s, "nodes projects remove").Confirm = nil }},
		{"consent flag undefined", "runs resolve: consent flag --ghost is not a bool flag", func(s *specFile) { rowOf(s, "runs resolve").Confirm.Flags = []string{"yes", "ghost"} }},
		{"resolve flag missing", "runs resolve: flag --reason is in the code but not in the spec", func(s *specFile) {
			r := rowOf(s, "runs resolve")
			for i, f := range r.Flags {
				if f.Name == "reason" {
					r.Flags = append(r.Flags[:i], r.Flags[i+1:]...)
					return
				}
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := cloneSpec(t, base)
			c.mutate(&s)
			for _, p := range specProblems(s) {
				if strings.HasPrefix(p, c.want) || strings.Contains(p, c.want) {
					return
				}
			}
			t.Fatalf("no problem %q in %v", c.want, specProblems(s))
		})
	}
}

// TestUnclassifiedKeyFails: a table key without a surfaceClasses entry is a problem, so a new command is classified.
func TestUnclassifiedKeyFails(t *testing.T) {
	saved := surfaceClasses["plan"]
	delete(surfaceClasses, "plan")
	defer func() { surfaceClasses["plan"] = saved }()
	for _, p := range specProblems(loadSpec(t)) {
		if strings.HasPrefix(p, "plan: no surfaceClasses entry") {
			return
		}
	}
	t.Fatal("an unclassified key passed")
}

// TestSurfaceSubsMatchRegistries: the positional subcommands a dispatcher row reaches are exactly the registered ones,
// so a new `artifacts <sub>` cannot ship unclassified.
func TestSurfaceSubsMatchRegistries(t *testing.T) {
	var got, want []string
	for k := range artifactsSubs {
		got = append(got, k)
	}
	for k := range surfaceClasses["artifacts"].Subs {
		want = append(want, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("artifacts subcommands %v differ from the classified %v", got, want)
	}
}

// TestRunsResolveDeclarationMatchesParser: every flag resolveFlagSet declares is accepted by the hand parser.
func TestRunsResolveDeclarationMatchesParser(t *testing.T) {
	resolveFlagSet().VisitAll(func(f *flag.Flag) {
		args := []string{"att", "--idempotency-key", "k"}
		switch f.Name {
		case "idempotency-key", "rerun":
			args = append(args, "--rerun")
		case "abandon":
			args = append(args, "--abandon")
		case "reason":
			args = append(args, "--rerun", "--reason", "v")
		default:
			args = append(args, "--rerun", "--"+f.Name)
		}
		if _, err := parseResolve(args); err != nil {
			t.Errorf("parseResolve rejects declared flag --%s: %v", f.Name, err)
		}
	})
}
