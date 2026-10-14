package config

// Purpose: load and validate the authored `.nself/ci.yaml` (Epic D7,
// contract:ci.pipeline-config v1).
// Inputs:  a file path, or a name and bytes.
// Outputs: the decoded model.PipelineConfig, line numbers per JSON pointer and
// loader warnings; or an *Error (E620) naming file:line and the JSON pointer of
// every problem, raised before any job can run.
// Constraints: yaml.v3 parses the document into nodes (so each problem keeps
// its line); the node tree becomes JSON, the generated schema validates it
// and a strict JSON decode (unknown fields refused, the JSON form of yaml.v3
// KnownFields) fills the model, because model types carry json tags only.
// Exactly one document, string keys only, no duplicate keys, no merge keys.
// Secrets are references (`secrets: [class/name]`, schema pattern), never
// literal values.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// FileName is the authored config path under a repo root.
const FileName = ".nself/ci.yaml"

const schemaName = "ci-pipeline-config.v1.schema.json"

// Problem is one located defect in a config document.
type Problem struct {
	File    string
	Line    int
	Pointer string
	Message string
}

func (p Problem) String() string {
	out := p.File
	if p.Line > 0 {
		out += ":" + strconv.Itoa(p.Line)
	}
	if p.Pointer != "" {
		out += ": " + p.Pointer
	}
	return out + ": " + p.Message
}

// Error is the E620 failure of an invalid config.
type Error struct {
	Code     string
	Problems []Problem
}

func (e *Error) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return e.Code + ": " + strings.Join(lines, "\n"+e.Code+": ")
}

// Warning is a non-fatal loader finding.
type Warning struct {
	Code    string `json:"code"`
	Line    int    `json:"line,omitempty"`
	Pointer string `json:"pointer,omitempty"`
	Message string `json:"message"`
}

// Loaded is a validated config with its provenance.
type Loaded struct {
	Path     string
	Config   model.PipelineConfig
	Warnings []Warning
	Lines    map[string]int
}

// Load reads and validates the config file at path.
func Load(path string) (*Loaded, error) {
	data, err := readRegular(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, &Error{Code: CodeInvalid, Problems: []Problem{{File: path, Message: err.Error()}}}
	}
	return Parse(path, data)
}

// Parse validates a config document. A validation failure is an *Error.
func Parse(name string, data []byte) (*Loaded, error) {
	fail := func(ps ...Problem) (*Loaded, error) {
		for i := range ps {
			ps[i].Pointer, ps[i].Message = printable(ps[i].Pointer), printable(ps[i].Message)
		}
		return nil, &Error{Code: CodeInvalid, Problems: ps}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return fail(Problem{File: name, Message: "empty document: version is required"})
		}
		return fail(yamlProblem(name, err))
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		line := extra.Line
		if len(extra.Content) > 0 {
			line = extra.Content[0].Line
		}
		return fail(Problem{File: name, Line: line, Message: "more than one YAML document"})
	}
	w := &walker{file: name, lines: map[string]int{}, budget: maxNodes, bytes: maxBytes}
	value := w.value(doc.Content[0], "", 0)
	lines := w.lines
	if len(w.probs) > 0 {
		return fail(w.probs...)
	}
	if _, ok := value.(map[string]any); !ok {
		return fail(Problem{File: name, Line: doc.Content[0].Line, Message: "the document must be a mapping"})
	}
	js, err := json.Marshal(value)
	if err != nil {
		return fail(Problem{File: name, Message: err.Error()})
	}
	if err := validateSchema(js); err != nil {
		ptr, msg := locate(err, value, schemaTree)
		return fail(Problem{File: name, Line: lineFor(lines, ptr), Pointer: ptr, Message: msg})
	}
	var out Loaded
	strict := json.NewDecoder(bytes.NewReader(js))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&out.Config); err != nil {
		return fail(decodeProblem(name, err, lines))
	}
	if probs := semantic(name, out.Config, lines); len(probs) > 0 {
		return fail(probs...)
	}
	out.Path, out.Lines = name, lines
	out.Warnings = envPresentWarnings(out.Config, lines)
	return &out, nil
}

var (
	schemaOnce     sync.Once
	schemaTree     *jsonschema.Schema
	schemaResolved *jsonschema.Resolved
	schemaErr      error
)

func validateSchema(doc []byte) error {
	schemaOnce.Do(func() {
		all, err := model.Schemas()
		if err != nil {
			schemaErr = err
			return
		}
		var tree jsonschema.Schema
		if err := json.Unmarshal(all[schemaName], &tree); err != nil {
			schemaErr = err
			return
		}
		if schemaResolved, schemaErr = tree.Resolve(nil); schemaErr == nil {
			schemaTree = &tree
		}
	})
	if schemaErr != nil {
		return schemaErr
	}
	var instance any
	if err := json.Unmarshal(doc, &instance); err != nil {
		return err
	}
	return schemaResolved.Validate(instance)
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func yamlProblem(name string, err error) Problem {
	p := Problem{File: name, Message: strings.TrimPrefix(err.Error(), "yaml: ")}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		p.Line, _ = strconv.Atoi(m[1])
	}
	return p
}

// lineFor returns the line of the longest known prefix of a JSON pointer.
func lineFor(lines map[string]int, ptr string) int {
	for {
		if l, ok := lines[ptr]; ok {
			return l
		}
		i := strings.LastIndex(ptr, "/")
		if i < 0 {
			return 0
		}
		ptr = ptr[:i]
	}
}

// printable replaces control characters so untrusted text from a config
// cannot forge log lines or drive a terminal.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

// decodeProblem locates a typed-decode failure (for example 1e30 for an int
// field) from the field path; the message names the Go type, never the value.
func decodeProblem(name string, err error, lines map[string]int) Problem {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		ptr := "/" + strings.ReplaceAll(te.Field, ".", "/")
		return Problem{File: name, Line: lineFor(lines, ptr), Pointer: ptr, Message: "value does not fit " + te.Type.String()}
	}
	return Problem{File: name, Message: "document does not decode into the config model"}
}
