package main

// Purpose: the code-side commitment behind commands.yaml (P7-CI-62, CI D31): for every key of the subcommand table the
//          lowest side-effect class its row may declare, the flags that raise a command above that class, and the
//          positional subcommands a dispatcher row reaches. cmd/surface_test.go fails when a row understates any of them.
// Inputs:  none at run time; the tables are read by tests and by nothing else.
// Constraints: every table key has an entry (an unlisted key fails the test, so a new command must be classified);
//          a Flag entry is satisfied by a cli_only flag (a machine caller cannot pass it) or by a row at least that
//          class; a Subs entry (a dispatcher such as `artifacts`) forces the row to its strongest subcommand,
//          because the machine caller chooses the positional.

import "flag"

type surfaceClass struct {
	Min   string            // lowest class of the key itself
	Flags map[string]string // flag name -> class a caller reaches with the flag
	Subs  map[string]string // positional subcommand -> class (dispatcher keys only)
}

var classRank = map[string]int{"read": 0, "write": 1, "remote": 2, "destructive": 3}

func cls(min string) surfaceClass { return surfaceClass{Min: min} }

// surfaceClasses holds one entry per table key. Reasons for the non-obvious ones sit beside them.
var surfaceClasses = map[string]surfaceClass{
	"artifacts": {Min: "read", Subs: map[string]string{ // reached through the `subcommand` positional
		"list": "read", "show": "read", "verify": "read", "get": "write", "sign": "write", "release": "write",
		"tools": "remote", "signer": "remote", "prune": "destructive", "key": "destructive", // key rotate
	}},
	"build": cls("remote"), "run": cls("remote"), "netproxy": cls("remote"),
	"config ignore add": cls("write"), "config migrate": cls("write"), "config show": cls("read"),
	"doctor":   {Min: "read", Flags: map[string]string{"alert": "remote"}}, // --alert sends through the notifier
	"evidence": cls("read"), "evidence verify": cls("read"), "explain": cls("read"), "forgejo": cls("read"),
	"logs": cls("read"), "plan": cls("read"),
	"nodes add": cls("remote"), "nodes drain": cls("remote"), "nodes enable": cls("remote"), "nodes pair": cls("remote"),
	"nodes provision": cls("remote"), "nodes rotate": cls("remote"), "nodes set": cls("remote"), "nodes test": cls("remote"),
	"nodes upgrade": cls("remote"), "nodes secrets grant": cls("remote"), "nodes secrets revoke": cls("remote"),
	"nodes label": cls("remote"), "nodes projects add": cls("remote"), "nodes projects remove": cls("remote"), // --coordinator mutates a remote
	"nodes forget-project": cls("destructive"), "nodes remove": cls("destructive"), "nodes revoke": cls("destructive"),
	"nodes sync":     {Min: "write", Flags: map[string]string{"cmd": "remote"}}, // --cmd runs a caller-supplied program
	"nodes discover": cls("read"), "nodes list": cls("read"), "nodes pools explain": cls("read"), "nodes pools list": cls("read"),
	"nodes pools show": cls("read"), "nodes projects list": cls("read"), "nodes show": cls("read"), "nodes status": cls("read"),
	"nodes verify": cls("read"),
	"policy pin":   cls("write"), "policy set": cls("write"), "policy unset": cls("write"), "policy protect": cls("remote"),
	"policy show": cls("read"), "policy usage": cls("read"),
	"providers": cls("read"), "providers list": cls("read"), "providers show": cls("read"),
	"providers authorize": cls("remote"), "providers test": cls("remote"), "providers revoke": cls("destructive"),
	"providers github init": cls("write"), "providers gitlab init": cls("write"), "providers usage import": cls("write"),
	"runs": cls("read"), "runs decisions": cls("read"), "runs follow": cls("read"), "runs list": cls("read"),
	"runs outbox": cls("read"), "runs pending": cls("read"), "runs queue": cls("read"), "runs stats": cls("read"),
	"runs wait":    cls("read"),
	"runs approve": cls("remote"), "runs cancel": cls("remote"), "runs reproduce": cls("remote"), "runs rerun": cls("remote"),
	"runs prune": cls("destructive"), "runs resolve": cls("destructive"), // resolve spends a release attempt that runs at most once
	"serve": cls("write"), "serve install": cls("write"), "serve watch": cls("remote"), "serve uninstall": cls("destructive"),
	"serve restore": cls("destructive"), "serve backup": cls("read"), "serve admin": cls("read"), "serve admin audit": cls("read"),
	"serve admin audit checkpoint": cls("write"), "serve admin cert": cls("write"),
	"serve admin roles": cls("destructive"), "serve admin token": cls("destructive"),
}

// runsSubFlags are the Flags constructors of `runs <sub>` handlers that parse their own argv, so the flag check
// reaches them; a `runs` row whose handler has no constructor here is not flag-checked.
var runsSubFlags = map[string]func() *flag.FlagSet{
	"resolve": func() *flag.FlagSet { return resolveFlagSet(nil) },
	"rerun":   func() *flag.FlagSet { return newRerunFlagSet(&rerunFlags{}) },
}
