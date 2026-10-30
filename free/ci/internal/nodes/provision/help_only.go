package provision

// Purpose: the flag sets of `nodes provision` and `nodes verify`, shared by the commands and by
//   HelpOnly, which answers a help request the way core does without running anything.
// Inputs:  argv after the subcommand key that asks for help (-h or --help).
// Outputs: the captured core help on stdout and exit 0, or core's "Error: ..." and exit 1 when a
//   flag before the help token does not parse (core's pflag reports the first parse error).
// Constraints: HelpOnly never touches a host or the registry; a help token taken as a flag's value
//   (`--token --help`) still prints help, because a help request must never run the command.

import "fmt"

// provisionFlagDefs are the `nodes provision` flags the command reads.
type provisionFlagDefs struct {
	host, sshKey, instances, installRoot, githubURL, labels, token, asJSON *flagDef
}

// provisionFlags adds the `nodes provision` flags to fs, with core's defaults.
func provisionFlags(fs *flagSet) provisionFlagDefs {
	f := provisionFlagDefs{
		host: fs.add("host", kindSlice), sshKey: fs.add("ssh-key", kindString), instances: fs.add("instances", kindInt),
		installRoot: fs.add("install-root", kindString), githubURL: fs.add("github-url", kindString),
		labels: fs.add("labels", kindSlice), token: fs.add("token", kindString), asJSON: fs.add("json", kindBool),
	}
	f.instances.num = 1
	fs.add("no-monorepo", kindBool)
	fs.add("no-deprecation-warnings", kindBool)
	return f
}

// verifyFlagDefs are the `nodes verify` flags the command reads.
type verifyFlagDefs struct{ host, sshKey, asJSON *flagDef }

// verifyFlags adds the `nodes verify` flags to fs.
func verifyFlags(fs *flagSet) verifyFlagDefs {
	f := verifyFlagDefs{host: fs.add("host", kindSlice), sshKey: fs.add("ssh-key", kindString), asJSON: fs.add("json", kindBool)}
	fs.add("no-monorepo", kindBool)
	fs.add("no-deprecation-warnings", kindBool)
	return f
}

// HelpOnly answers `nodes <name> ... --help` for name "provision" or "verify": the flags parse first
// (an error wins, exit 1, as in core), then the captured help prints (exit 0). It runs nothing.
func HelpOnly(name string, args []string, d Deps) int {
	fs := newFlagSet()
	if name == "provision" {
		provisionFlags(fs)
	} else {
		verifyFlags(fs)
	}
	if err := fs.parse(args); err != nil {
		return finish(d, err)
	}
	txt, _ := Help(name)
	_, _ = fmt.Fprint(d.Out, txt)
	return 0
}
