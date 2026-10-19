package bootstrap

// Purpose: choose the family of a node and, for a macOS node nobody registered a family for, print the
// brew commands the operator runs by hand (today's behaviour; P7-NODE-31 registers the darwin family).
// Inputs:  a RunFunc bound to the node.
// Outputs: the matching Family, or (nil, false) with the OS key.
// Constraints: the answers come from `uname -s` and /etc/os-release of the node and are matched against
//          registered names only; nothing from the node reaches a script.

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// DarwinBrew are the commands printed for a macOS node without a registered family.
var DarwinBrew = []string{"brew install git", "brew install coreutils"}

// OSKeys returns the keys to try against Lookup, most specific first: for Linux the os-release ID and
// then each ID_LIKE word, for macOS "darwin".
func OSKeys(ctx context.Context, run RunFunc) ([]string, error) {
	out, err := run(ctx, "uname", "-s")
	if err != nil {
		return nil, NewError(ReasonStep, "cannot read the node's operating system: %s", Detail(err))
	}
	switch strings.TrimSpace(lastLine(out)) {
	case "Darwin":
		return []string{"darwin"}, nil
	case "Linux":
		rel, rerr := run(ctx, "cat", "/etc/os-release")
		if rerr != nil {
			return nil, NewError(ReasonStep, "cannot read /etc/os-release on the node: %s", Detail(rerr))
		}
		return parseOSRelease(rel), nil
	}
	return nil, NewError(ReasonUnsupported, "unsupported operating system %q", Detail(fmt.Errorf("%s", lastLine(out))))
}

// parseOSRelease lists ID, then the ID_LIKE words, lower case.
func parseOSRelease(text string) []string {
	var id string
	var like []string
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.ToLower(strings.Trim(v, `"'`))
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			like = strings.Fields(v)
		}
	}
	var keys []string
	if id != "" {
		keys = append(keys, id)
	}
	return append(keys, like...)
}

// Detect finds the registered family of the node. When none matches, ok is false and keys says what
// was tried.
func Detect(ctx context.Context, run RunFunc) (f Family, keys []string, ok bool, err error) {
	keys, err = OSKeys(ctx, run)
	if err != nil {
		return nil, nil, false, err
	}
	for _, k := range keys {
		if f, ok = Lookup(k); ok {
			return f, keys, true, nil
		}
	}
	return nil, keys, false, nil
}

// PrintDarwinHint tells the operator what to run on a macOS node without a registered family.
func PrintDarwinHint(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "This macOS node has no system-install family; run these on it as an administrator, then add it without --system:")
	for _, c := range DarwinBrew {
		_, _ = fmt.Fprintln(w, "  "+c)
	}
}
