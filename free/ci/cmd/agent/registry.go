package main

// Purpose: the agent binary's extension points (review F6). Other tickets add
// subcommands (join, pull, advertise, upgrade, verify-release, job-shim) and
// PreStart hooks from their own files, so main.go never needs editing.
// Inputs:  RegisterSubcommand and RegisterPreStart calls, normally from init.
// Outputs: lookup for main, and RunPreStart for serve, join and pull.
// Constraints: a name registers once; PreStart hooks run in registration order
// and the first error stops the command before it reads or writes anything.

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
)

// IO is a subcommand's standard streams.
type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Subcommand runs one command and returns its exit code (0 ok, 1 failed, 2 usage).
type Subcommand func(ctx context.Context, args []string, sio IO) int

// PreStartHook vets the host before serve, join or pull begin (P7-NODE-14
// registers the production guard). A non-nil error stops the command.
type PreStartHook func(ctx context.Context, command string) error

type command struct {
	summary string
	run     Subcommand
}

type preStart struct {
	name string
	run  PreStartHook
}

var (
	regMu    sync.RWMutex
	commands = map[string]command{}
	prestart []preStart
)

// RegisterSubcommand adds a subcommand. An empty or duplicate name is an error.
func RegisterSubcommand(name, summary string, run Subcommand) error {
	regMu.Lock()
	defer regMu.Unlock()
	if name == "" || run == nil {
		return fmt.Errorf("subcommand needs a name and a function")
	}
	if _, ok := commands[name]; ok {
		return fmt.Errorf("duplicate subcommand %q", name)
	}
	commands[name] = command{summary, run}
	return nil
}

// RegisterPreStart adds a PreStart hook. An empty or duplicate name is an error.
func RegisterPreStart(name string, run PreStartHook) error {
	regMu.Lock()
	defer regMu.Unlock()
	if name == "" || run == nil {
		return fmt.Errorf("pre-start hook needs a name and a function")
	}
	for _, p := range prestart {
		if p.name == name {
			return fmt.Errorf("duplicate pre-start hook %q", name)
		}
	}
	prestart = append(prestart, preStart{name, run})
	return nil
}

// RunPreStart runs every hook for command in order and stops at the first error.
func RunPreStart(ctx context.Context, command string) error {
	regMu.RLock()
	hooks := append([]preStart(nil), prestart...)
	regMu.RUnlock()
	for _, h := range hooks {
		if err := h.run(ctx, command); err != nil {
			return fmt.Errorf("pre-start %s: %w", h.name, err)
		}
	}
	return nil
}

func lookup(name string) (Subcommand, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := commands[name]
	return c.run, ok
}

// usageLines lists the registered subcommands, sorted.
func usageLines() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	lines := make([]string, len(names))
	for i, n := range names {
		lines[i] = fmt.Sprintf("  %-14s %s", n, commands[n].summary)
	}
	return lines
}

// must panics on a registration error: a duplicate name is a build mistake.
func must(err error) {
	if err != nil {
		panic(err)
	}
}
