package redact

// Purpose: stop job output from steering the log consumer. Forge workflow
// commands (::add-mask::, ::set-output, ::stop-commands::, ##vso[ ...) and
// terminal escape sequences (OSC 8/52, other OSC, DCS/APC/PM/SOS, non-colour
// CSI, lone ESC, UTF-8 encoded C1 controls) are rewritten into inert text.
// Inputs:  a buffer and the byte before it. Outputs: edits and the number of
// trailing bytes that may be the start of a sequence.
// Constraints: SGR colour sequences (ESC [ digits ; : m) pass through; command
// names are matched only when the byte before `::` is not a letter, digit or
// underscore, so `std::error::Error` is unchanged.

import "strings"

// edit is one replacement the neutraliser asks for.
type edit struct {
	s, e int
	with string
}

const (
	escMark     = "^["
	maxSGRBytes = 32
)

var commands = []string{
	"add-mask", "set-output", "save-state", "set-env", "add-path", "stop-commands", "debug", "warning", "error",
	"notice", "group", "endgroup", "echo", "add-matcher", "remove-matcher", "set-secret", "pause-commands",
}

// isIdent reports a byte that can continue an identifier.
func isIdent(c byte) bool { return isAlnum(c) || c == '_' }

// isCmdChar reports a byte of a workflow command name.
func isCmdChar(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' }

// cmdState classifies the name that follows `::`: a full command needing the
// next byte, a command with a terminator, a prefix of one, or not a command.
func cmdState(rest []byte) (isCmd, partial bool) {
	n := 0
	for n < len(rest) && isCmdChar(rest[n]) {
		n++
	}
	name := strings.ToLower(string(rest[:n])) // runners match command names case-insensitively
	for _, c := range commands {
		if c == name {
			if n == len(rest) {
				return false, true
			}
			return rest[n] == ':' || rest[n] == ' ', false
		}
	}
	if n == len(rest) {
		for _, c := range commands {
			if len(c) > n && c[:n] == name {
				return false, true
			}
		}
	}
	return false, false
}

// neutralise returns the edits for b and how many trailing bytes to hold back.
func neutralise(b []byte, prev byte, final bool, out []edit) (edits []edit, hold int) {
	held := func(i int) {
		if !final && len(b)-i > hold {
			hold = len(b) - i
		}
	}
	for i := 0; i < len(b); i++ {
		switch c := b[i]; c {
		case 0x1b:
			end, sgr, incomplete := escape(b, i)
			if incomplete && !final {
				held(i)
				return out, hold
			}
			if sgr {
				i = end - 1
				continue
			}
			out = append(out, edit{i, i + 1, escMark})
		case 0xc2:
			if i+1 == len(b) {
				held(i)
			} else if b[i+1] >= 0x80 && b[i+1] <= 0x9f {
				out = append(out, edit{i, i + 2, escMark})
				i++
			}
		case ':':
			p := prev
			if i > 0 {
				p = b[i-1]
			}
			if isIdent(p) {
				continue
			}
			if i+1 == len(b) {
				held(i)
				continue
			}
			if b[i+1] != ':' {
				continue
			}
			isCmd, partial := cmdState(b[i+2:])
			if partial {
				held(i)
			} else if isCmd {
				out = append(out, edit{i, i + 2, "[::]"})
				i++
			}
		case '#':
			if i+1 == len(b) {
				held(i)
				continue
			}
			if b[i+1] != '#' {
				continue
			}
			rest := b[i+2:]
			switch {
			case len(rest) >= 1 && rest[0] == '[':
				out = append(out, edit{i, i + 2, "[##]"})
			case len(rest) >= 4 && strings.EqualFold(string(rest[:4]), "vso["):
				out = append(out, edit{i, i + 2, "[##]"})
			case len(rest) < 4 && strings.EqualFold(string(rest), "vso"[:len(rest)]):
				held(i)
			}
			if len(out) > 0 && out[len(out)-1].s == i {
				i++
			}
		}
	}
	return out, hold
}

// escape classifies the sequence at b[i] == ESC. sgr is a complete colour
// sequence ending before end; incomplete means b ends inside a possible SGR.
func escape(b []byte, i int) (end int, sgr, incomplete bool) {
	if i+1 == len(b) {
		return 0, false, true
	}
	if b[i+1] != '[' {
		return 0, false, false
	}
	j := i + 2
	for j < len(b) && (b[j] >= '0' && b[j] <= '9' || b[j] == ';' || b[j] == ':') {
		j++
	}
	if j == len(b) {
		return 0, false, j-i <= maxSGRBytes
	}
	if b[j] == 'm' && j-i <= maxSGRBytes {
		return j + 1, true, false
	}
	return 0, false, false
}
