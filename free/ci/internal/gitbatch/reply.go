package gitbatch

// Purpose: the wire format of `git cat-file --batch-command`: one request line, one reply header
// (`<oid> <type> <size>`, `<name> missing` or `<name> ambiguous`) and, for `contents`, the body and a newline.
// Inputs:  a proc and a request. Outputs: the Object and its bytes.
// Constraints: any other shape is ErrMalformed; a body over Options.MaxBytes is ErrUnavailable because the
//          stream cannot be skipped without reading it.

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// exchange sends one request and reads its reply.
func (s *Session) exchange(p *proc, verb, name string) (Object, []byte, error) {
	if _, err := io.WriteString(p.stdin, verb+" "+name+"\n"); err != nil {
		return Object{}, nil, fmt.Errorf("write: %w", err)
	}
	line, err := p.rd.ReadString('\n')
	if err != nil {
		return Object{}, nil, fmt.Errorf("read: %w", err)
	}
	f := strings.Fields(strings.TrimSuffix(line, "\n"))
	if n := len(f); n >= 2 && f[n-1] == "missing" {
		return Object{}, nil, ErrNotFound
	} else if n >= 2 && f[n-1] == "ambiguous" {
		return Object{}, nil, ErrAmbiguous
	}
	if len(f) != 3 || !validOID(f[0]) || !validType(f[1]) {
		return Object{}, nil, fmt.Errorf("%w: %q", ErrMalformed, strings.TrimSpace(line))
	}
	size, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || size < 0 {
		return Object{}, nil, fmt.Errorf("%w: size in %q", ErrMalformed, strings.TrimSpace(line))
	}
	obj := Object{OID: f[0], Type: f[1], Size: size}
	if verb == "info" {
		return obj, nil, nil
	}
	if size > s.opt.MaxBytes {
		return Object{}, nil, fmt.Errorf("%w: object of %d bytes", ErrUnavailable, size) // cannot skip it without reading it
	}
	buf := make([]byte, size+1)
	if _, err := io.ReadFull(p.rd, buf); err != nil {
		return Object{}, nil, fmt.Errorf("read body: %w", err)
	}
	if buf[size] != '\n' {
		return Object{}, nil, fmt.Errorf("%w: object body not closed by a newline", ErrMalformed)
	}
	return obj, buf[:size], nil
}

func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func validType(t string) bool { return t == "blob" || t == "tree" || t == "commit" || t == "tag" }
