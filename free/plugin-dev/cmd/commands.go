package main

// Purpose: the subcommand table of nself-plugin-dev: names, aliases, flags with
// core's defaults, and the handler of each. Mirrors the cobra declarations in
// core cmd/commands/plugin_{new,dev,debug,link,test_cmd}.go.
// Inputs:  none. Outputs: the commands slice and lookup.
// Constraints: flag names, defaults and kinds equal core's; the help text of
// each command is the captured core text (internal/surface), not retyped here.

type command struct {
	name       string
	aliases    []string
	short      string
	deprecated string
	flags      []flagSpec
	run        func(*parsed) error
}

var scaffoldFlags = []flagSpec{
	{"template", kString, "go"},
	{"tier", kString, "free"},
	{"bundle", kString, ""},
	{"description", kString, ""},
	{"author", kString, ""},
	{"category", kString, "custom"},
	{"min-cli", kString, "1.0.9"},
	{"min-sdk", kString, "0.1.0"},
	{"port", kInt, "8080"},
	{"out", kString, ""},
	{"force", kBool, "false"},
	{"tenancy", kString, ""},
	{"no-interactive", kBool, "false"},
}

var commands = []*command{
	{name: "init", aliases: []string{"scaffold"}, short: "Scaffold a new plugin project", flags: scaffoldFlags, run: runInit},
	{name: "new", short: "Scaffold a new plugin project (deprecated: use 'init')", deprecated: "use 'nself plugin-dev init' instead", flags: scaffoldFlags, run: runInit},
	{name: "dev", short: "Start a plugin in development mode with hot-reload", flags: []flagSpec{
		{"no-link", kBool, "false"}, {"debug", kBool, "false"}, {"entrypoint", kString, "./cmd"},
	}, run: runDev},
	{name: "debug", short: "Attach a dlv debugger to a running plugin process", flags: []flagSpec{
		{"port", kInt, "0"}, {"port-only", kBool, "false"},
	}, run: runDebug},
	{name: "link", short: "Register a local plugin directory as a shadow override", flags: []flagSpec{
		{"host", kBool, "false"}, {"list", kBool, "false"},
	}, run: runLink},
	{name: "unlink", short: "Remove a local plugin shadow, restoring the registry version", run: runUnlink},
	{name: "test", short: "Run a plugin's test suite (unit + smoke install/uninstall)", flags: []flagSpec{
		{"phase", kString, "both"}, {"host", kBool, "false"}, {"no-cleanup", kBool, "false"},
	}, run: runPluginTest},
}

// lookup finds a command by name or alias.
func lookup(name string) (*command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
		for _, a := range c.aliases {
			if a == name {
				return c, true
			}
		}
	}
	return nil, false
}
