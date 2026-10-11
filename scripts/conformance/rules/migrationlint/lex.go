// lex.go: a small PostgreSQL lexer for the migration lint (P7 D-0346). It understands -- comments, nested /* */,
// '...' strings (” escapes), E'...' strings (backslash escapes), "quoted idents" and $tag$...$tag$ bodies, and
// keeps the 1-based start line of every token. Comments are dropped; string contents are kept apart from code.
package main

import "strings"

type kind int

const (
	kWord  kind = iota // identifier, keyword or number; U is upper-cased
	kQID               // "quoted ident"; U is the raw name
	kStr               // '...' string; U is the unescaped content
	kBody              // $tag$...$tag$; U is the body, BodyLine the line it starts on
	kPunct             // one punctuation byte
)

type tok struct {
	K        kind
	U        string
	Line     int
	BodyLine int
}

func isIDStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}
func isIDPart(c byte) bool { return isIDStart(c) || (c >= '0' && c <= '9') || c == '$' }

// lex tokenises src; line is the 1-based line number of its first byte.
func lex(src string, line int) []tok {
	var out []tok
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case c == '-' && i+1 < n && src[i+1] == '-':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth := 1
			i += 2
			for i < n && depth > 0 {
				switch {
				case src[i] == '/' && i+1 < n && src[i+1] == '*':
					depth++
					i += 2
				case src[i] == '*' && i+1 < n && src[i+1] == '/':
					depth--
					i += 2
				default:
					if src[i] == '\n' {
						line++
					}
					i++
				}
			}
		case c == '\'':
			s, j, nl := scanString(src, i, false)
			out = append(out, tok{K: kStr, U: s, Line: line})
			line += nl
			i = j
		case c == '"':
			var b strings.Builder
			j, start := i+1, line
			for j < n {
				if src[j] == '"' {
					if j+1 < n && src[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					break
				}
				if src[j] == '\n' {
					line++
				}
				b.WriteByte(src[j])
				j++
			}
			out = append(out, tok{K: kQID, U: b.String(), Line: start})
			i = j + 1
		case c == '$':
			j := i + 1
			if j < n && src[j] >= '0' && src[j] <= '9' { // $1 parameter
				for j < n && src[j] >= '0' && src[j] <= '9' {
					j++
				}
				out = append(out, tok{K: kWord, U: src[i:j], Line: line})
				i = j
				break
			}
			for j < n && isIDPart(src[j]) && src[j] != '$' {
				j++
			}
			if j < n && src[j] == '$' {
				delim := src[i : j+1]
				end := strings.Index(src[j+1:], delim)
				body, next := "", n
				if end < 0 {
					body = src[j+1:]
				} else {
					body, next = src[j+1:j+1+end], j+1+end+len(delim)
				}
				out = append(out, tok{K: kBody, U: body, Line: line, BodyLine: line})
				line += strings.Count(body, "\n")
				i = next
				break
			}
			out = append(out, tok{K: kPunct, U: "$", Line: line})
			i++
		case isIDStart(c):
			j := i + 1
			for j < n && isIDPart(src[j]) {
				j++
			}
			if j < n && src[j] == '\'' && (src[i:j] == "E" || src[i:j] == "e") {
				s, k, nl := scanString(src, j, true)
				out = append(out, tok{K: kStr, U: s, Line: line})
				line += nl
				i = k
				break
			}
			out = append(out, tok{K: kWord, U: strings.ToUpper(src[i:j]), Line: line})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && isIDPart(src[j]) {
				j++
			}
			out = append(out, tok{K: kWord, U: src[i:j], Line: line})
			i = j
		default:
			out = append(out, tok{K: kPunct, U: string(c), Line: line})
			i++
		}
	}
	return out
}

// scanString reads the string whose opening quote is at src[i]; it returns the content, the index after the closing
// quote and the newlines crossed. esc enables backslash escapes (E'...').
func scanString(src string, i int, esc bool) (string, int, int) {
	var b strings.Builder
	nl := 0
	j := i + 1
	for j < len(src) {
		ch := src[j]
		if esc && ch == '\\' && j+1 < len(src) {
			if src[j+1] == '\n' {
				nl++
			}
			b.WriteByte(src[j+1])
			j += 2
			continue
		}
		if ch == '\'' {
			if j+1 < len(src) && src[j+1] == '\'' {
				b.WriteByte('\'')
				j += 2
				continue
			}
			return b.String(), j + 1, nl
		}
		if ch == '\n' {
			nl++
		}
		b.WriteByte(ch)
		j++
	}
	return b.String(), j, nl
}

// lexFile tokenises a whole file: a leading UTF-8 byte order mark is dropped (it would glue itself to the first keyword).
func lexFile(src string) []tok { return lex(strings.TrimPrefix(src, "\xef\xbb\xbf"), 1) }

// split cuts toks into statements at ';' (string and body contents are single tokens, so only real separators split).
func split(toks []tok) [][]tok {
	var out [][]tok
	var cur []tok
	for _, t := range toks {
		if t.K == kPunct && t.U == ";" {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}
