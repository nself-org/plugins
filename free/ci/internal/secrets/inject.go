package secrets

// Purpose: the three env-only delivery forms (Epic D5). Values never touch a file, an argv or a URL.
// Inputs:  the resolved name -> value map (names are already validated refs).
// Outputs: process environment entries; container `-e NAME` argv entries plus the docker CLI's own
// environment (the value is read from there, so it is not in `docker run`'s argv); Tart script lines
// written to the SSH session's stdin.
// Constraints: names are re-validated here; a Tart value is single-quoted for POSIX sh and a value with a
// NUL byte is refused; everything returned is a fresh copy the caller owns.

import (
	"bytes"
	"sort"
	"strings"
)

func sortedNames(values map[string]string) ([]string, error) {
	names := make([]string, 0, len(values))
	for n := range values {
		if !namePattern.MatchString(n) {
			return nil, errRef("invalid environment name %q", echo(n))
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// ProcessEnv returns NAME=value entries for a process job, sorted by name.
func ProcessEnv(values map[string]string) ([]string, error) {
	names, err := sortedNames(values)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n + "=" + values[n]
	}
	return out, nil
}

// ContainerArgs returns the `-e NAME` pairs for `docker run`: names only, never values.
func ContainerArgs(values map[string]string) ([]string, error) {
	names, err := sortedNames(values)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, 2*len(names))
	for _, n := range names {
		out = append(out, "-e", n)
	}
	return out, nil
}

// ContainerCLIEnv returns the NAME=value entries to add to the docker CLI process's environment, where
// `-e NAME` finds them.
func ContainerCLIEnv(values map[string]string) ([]string, error) { return ProcessEnv(values) }

// TartStdin returns the script lines the SSH session reads on stdin: `export NAME='value'` per secret.
func TartStdin(values map[string]string) ([]byte, error) {
	names, err := sortedNames(values)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	for _, n := range names {
		v := values[n]
		if strings.ContainsRune(v, 0) {
			zero(b.Bytes())
			return nil, errRef("%s cannot be delivered over stdin: the value contains a NUL byte", n)
		}
		b.WriteString("export ")
		b.WriteString(n)
		b.WriteString("='")
		b.WriteString(strings.ReplaceAll(v, "'", `'\''`))
		b.WriteString("'\n")
	}
	return b.Bytes(), nil
}
