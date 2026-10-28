package main

// Purpose: the per-type schema registry of schemagen. A file that owns one schema registers it from
// init() (a name and a func that returns the bytes); main walks the registry in name order and writes
// each result. Nothing is written until main runs.
// Constraints: a name is registered once (a second register panics at start-up, so two types cannot
// silently share a file); a func returns deterministic bytes.

import (
	"fmt"
	"sort"
)

// schemaFunc returns the bytes of one schema file.
type schemaFunc func() ([]byte, error)

var registry = map[string]schemaFunc{}

// register adds the schema written to schemas/<name>.
func register(name string, f schemaFunc) {
	if _, dup := registry[name]; dup {
		panic("schemagen: schema registered twice: " + name)
	}
	registry[name] = f
}

// registered returns every registered schema by file name.
func registered() (map[string][]byte, error) {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make(map[string][]byte, len(names))
	for _, n := range names {
		b, err := registry[n]()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out[n] = b
	}
	return out, nil
}
