package doctor

// Purpose: the section registry. P7-CI-43 (toolchain drift), P7-NODE
// (`--nodes`), P7-HOST (`--providers`) and P7-TRUST (isolation last-verified)
// register a Section from their own packages' init(); this package never
// imports them, so adding a section never edits internal/doctor.
// Inputs:  Section values; the selectors the caller asked for.
// Outputs: the checks of every selected section, each run under a time budget.
// Constraints: a section id prefix is its owner's (CI-DN… nodes, CI-DP…
// providers, CI-DT… trust); ids CI-D001–CI-D099 belong to this package. A
// section that overruns its budget yields one warning and is abandoned.

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Selectors a Section can bind to. "" runs on every doctor invocation.
const (
	SelectAlways    = ""
	SelectNodes     = "nodes"
	SelectProviders = "providers"
)

// SectionBudget is the time one section may take.
const SectionBudget = 10 * time.Second

// Env is what a section may use: the repository root, the injectable process
// runner (the exec funnel) and the environment reader.
type Env struct {
	Root    string
	Capture CaptureFunc
	Getenv  func(string) string
}

// Section adds checks to the report.
type Section struct {
	// Name identifies the section; it is unique.
	Name string
	// Selector is SelectAlways, SelectNodes or SelectProviders.
	Selector string
	// Run returns the section's checks. It must honour ctx.
	Run func(ctx context.Context, env Env) []Check
}

var (
	sectionMu sync.Mutex
	sections  []Section
)

// RegisterSection adds a section. A duplicate name or an unknown selector is a
// programming error and panics, as for the subcommand table.
func RegisterSection(s Section) {
	if s.Name == "" || s.Run == nil {
		panic("doctor: section needs a name and a Run function")
	}
	switch s.Selector {
	case SelectAlways, SelectNodes, SelectProviders:
	default:
		panic("doctor: unknown section selector " + s.Selector)
	}
	sectionMu.Lock()
	defer sectionMu.Unlock()
	for _, have := range sections {
		if have.Name == s.Name {
			panic("doctor: duplicate section " + s.Name)
		}
	}
	sections = append(sections, s)
}

// Registered returns the registered sections sorted by name.
func Registered() []Section {
	sectionMu.Lock()
	defer sectionMu.Unlock()
	out := append([]Section(nil), sections...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// noneRegistered is the placeholder check for a selector with no section.
var noneRegistered = map[string]Check{
	SelectNodes:     {ID: "CI-D090", Name: "nodes section", Status: Pass, Message: "none registered"},
	SelectProviders: {ID: "CI-D091", Name: "providers section", Status: Pass, Message: "none registered"},
}

// RunSections runs the always-on sections plus those whose selector is in
// want, in name order. A wanted selector with no section yields "none
// registered".
func RunSections(ctx context.Context, env Env, want map[string]bool, budget time.Duration) []Check {
	var out []Check
	have := map[string]bool{}
	for _, s := range Registered() {
		if s.Selector != SelectAlways && !want[s.Selector] {
			continue
		}
		have[s.Selector] = true
		out = append(out, runOne(ctx, s, env, budget)...)
	}
	for _, sel := range []string{SelectNodes, SelectProviders} {
		if want[sel] && !have[sel] {
			out = append(out, noneRegistered[sel])
		}
	}
	return out
}

// runOne runs one section in a goroutine so a section that ignores its
// context cannot hang the report.
func runOne(ctx context.Context, s Section, env Env, budget time.Duration) []Check {
	cctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	done := make(chan []Check, 1)
	go func() { done <- s.Run(cctx, env) }()
	select {
	case checks := <-done:
		return checks
	case <-cctx.Done():
		return []Check{{
			ID: "CI-D098", Name: "section " + s.Name, Status: Warn, Subject: s.Name,
			Message:     fmt.Sprintf("section %s did not finish within %s", s.Name, budget),
			Remediation: "Run the section's own command for details, or free the resource it waits on",
		}}
	}
}
