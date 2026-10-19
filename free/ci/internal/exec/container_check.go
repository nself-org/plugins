package exec

// Purpose: the backstop that refuses any planned container command that is not hardened.
// Inputs:  a profile's ContainerCommand and the ContainerRun it was built for.
// Outputs: nil, or an E613 error naming the first refused flag.
// Constraints: a flag allowlist with value checks, not a denylist, so a flag nobody listed
// (--privileged, host namespaces, --cap-add, --device, volumes, short flags) is refused.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// envFileStdin is how a planned container receives its environment: the CLI reads it from the
// pipe exec feeds, so no secret value touches disk or an argv (Epic D5).
const envFileStdin = "/dev/stdin"

// minJobUID is the first job UID (Epic D12: 65532 + slot).
const minJobUID = 65532

var (
	runtimeBinary = regexp.MustCompile(`^(docker|podman)$`)
	// resourceSignatures are what a job prints when the kernel refuses a fork.
	resourceSignatures = []string{"Resource temporarily unavailable", "can't fork", "Cannot fork", "pthread_create failed"}
	jobUser            = regexp.MustCompile(`^([1-9][0-9]*):([1-9][0-9]*)$`)
	attemptNetwork     = regexp.MustCompile(`^nself-ci-[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	memoryValue        = regexp.MustCompile(`^[1-9][0-9]{0,8}m$`)
)

// plannedFlags is the allowlist of flags a planned container may carry and whether each takes a
// separate value. Anything else (including every short flag) is refused, so a profile cannot
// add --privileged, a host namespace, --cap-add, --device or a volume by accident.
var plannedFlags = map[string]bool{
	"--rm": false, "--read-only": false, "--name": true, "--label": true, "--network": true, "--dns": true,
	"--cap-drop": true, "--security-opt": true, "--tmpfs": true, "--pids-limit": true, "--cpus": true,
	"--memory": true, "--memory-swap": true, "--user": true, "--env": true, "--runtime": true, "--userns": true,
	"--entrypoint": true, "--mount": true, "--workdir": true, "--env-file": true,
}

// checkContainerCommand refuses a planned command that is not a hardened `run` of run.Image.
// Only the flag region (before the image) is checked: the job command may carry any word.
func checkContainerCommand(c ContainerCommand, run ContainerRun) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	if !runtimeBinary.MatchString(c.Binary) {
		return bad("unknown runtime " + c.Binary)
	}
	if len(c.Args) == 0 || c.Args[0] != "run" {
		return bad("not a run command")
	}
	seen := map[string]bool{}
	named := map[string]bool{}
	i := 1
	for ; i < len(c.Args) && c.Args[i] != run.Image; i++ {
		name, value, hasValue := strings.Cut(c.Args[i], "=")
		takesValue, known := plannedFlags[name]
		if !strings.HasPrefix(name, "--") || !known {
			return bad("flag not allowed: " + c.Args[i])
		}
		if takesValue && !hasValue {
			if i+1 >= len(c.Args) {
				return bad("flag without value: " + name)
			}
			i++
			value = c.Args[i]
		} else if !takesValue && hasValue {
			return bad("flag takes no value: " + name)
		}
		if err := checkFlagValue(name, value, run); err != nil {
			return err
		}
		if name == "--name" && value != run.Name && value != run.Name+"-clean" {
			return bad("container name is not the attempt's: " + value)
		}
		seen[name+"="+value] = true
		named[name] = true
	}
	if i >= len(c.Args) {
		return bad("image not on the command line")
	}
	for _, want := range []string{"--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only="} {
		if !seen[want] {
			return bad("missing " + strings.TrimSuffix(want, "="))
		}
	}
	for _, want := range []string{"--user", "--network", "--pids-limit", "--memory", "--cpus", "--name", "--label"} { // no default user (root), network or limit
		if !named[want] {
			return bad("missing " + want)
		}
	}
	if !seen["--label=nself.ci.attempt="+run.AttemptID] {
		return bad("missing the attempt label")
	}
	return nil
}

// checkFlagValue constrains the flags whose value decides the isolation.
func checkFlagValue(name, value string, run ContainerRun) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	switch name {
	case "--cap-drop":
		if value != "ALL" {
			return bad("cap-drop must be ALL")
		}
	case "--security-opt":
		if value != "no-new-privileges" && value != "no-new-privileges=true" {
			return bad("security-opt " + value)
		}
	case "--network":
		if value != "none" && value != "bridge" && !attemptNetwork.MatchString(value) {
			return bad("network " + value)
		}
	case "--user":
		m := jobUser.FindStringSubmatch(value)
		if m == nil || m[1] != m[2] {
			return bad("user must be a numeric uid:uid, not root")
		}
		if n, _ := strconv.Atoi(m[1]); n < minJobUID {
			return bad("job user is below the job UID range")
		}
		if uid := strconv.Itoa(os.Getuid()); m[1] == uid || m[1] == strconv.Itoa(os.Geteuid()) {
			return bad("job user equals the agent user")
		}
	case "--env":
		if value != "HOME=/tmp" && value != "TMPDIR=/tmp" && !passedEnvName(value, run) {
			return bad("env value on argv: only HOME and TMPDIR may be set there, and declared secrets by name")
		}
	case "--runtime":
		if value != "runsc" {
			return bad("runtime " + value)
		}
	case "--userns":
		if value != "auto" {
			return bad("userns " + value)
		}
	case "--mount":
		for _, m := range run.Mounts {
			if m.ReadOnly && value == m.Spec() { // a read-only bind the executor accepted (checkMounts)
				return nil
			}
		}
		return checkMount(value, run.Workspace)
	case "--tmpfs":
		if !strings.HasPrefix(value, "/tmp:") && value != "/tmp" {
			return bad("tmpfs outside /tmp")
		}
	case "--env-file":
		if value != envFileStdin {
			return bad("env file must be read from stdin, never from disk")
		}
	case "--pids-limit":
		if n, err := strconv.Atoi(value); err != nil || n < 1 || n > 1<<20 {
			return bad("pids-limit must be a positive number")
		}
	case "--memory", "--memory-swap":
		if !memoryValue.MatchString(value) {
			return bad(name + " must be a positive size in megabytes")
		}
	case "--cpus":
		if f, err := strconv.ParseFloat(value, 64); err != nil || f <= 0 || f > 1024 {
			return bad("cpus must be positive")
		}
	}
	return nil
}

// passedEnvName reports whether value is exactly the name of a secret exec hands to the
// runtime CLI through its environment (a bare name, never NAME=value).
func passedEnvName(value string, run ContainerRun) bool {
	for _, n := range run.PassEnv {
		if value == n {
			return true
		}
	}
	return false
}

// checkMount allows exactly one bind of the attempt workspace onto /repo: the keys are
// type, src and dst, plus an optional readonly=true that only narrows the job's access (no
// alias such as source or target, no duplicate, nothing else).
func checkMount(spec, workspace string) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	fields := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return bad("mount option without a value: " + part)
		}
		if _, dup := fields[k]; dup {
			return bad("duplicate mount key " + k)
		}
		fields[k] = v
	}
	ro, hasRO := fields["readonly"]
	if hasRO && ro != "true" {
		return bad("mount readonly must be true")
	}
	want := 3
	if hasRO {
		want = 4
	}
	if len(fields) != want || fields["type"] != "bind" || fields["src"] != workspace || fields["dst"] != "/repo" {
		return bad("mount is not the job root bound at /repo")
	}
	return nil
}
