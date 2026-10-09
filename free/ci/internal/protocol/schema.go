package protocol

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// Schema returns deterministic JSON Schema for the v1 frame and every body.
func Schema() ([]byte, error) {
	types := map[string]any{
		"hello": Hello{}, "welcome": Welcome{}, "reject": Reject{},
		"heartbeat": Heartbeat{}, "lease": Lease{}, "secrets": Secrets{},
		"chunk": Chunk{}, "log": Log{}, "artifact-ref": ArtifactRef{},
		"result": Result{}, "cancel": Cancel{}, "drain": Drain{},
		"upgrade": Upgrade{}, "ack": Ack{}, "digest": Digest{},
		"offset": Offset{}, "resume": Resume{}, "cache-ref": CacheRef{},
	}
	kinds := make([]string, 0, len(types))
	branches := make([]any, 0, len(types))
	for kind := range types {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		body := types[kind]
		bodySchema := typeSchema(reflect.TypeOf(body))
		bodyProps := bodySchema["properties"].(map[string]any)
		switch kind {
		case "chunk", "digest", "offset":
			bodyProps["stream"] = map[string]any{"enum": []string{"checkout", "artifact", "cache"}}
		case "lease":
			bodyProps["epoch"] = map[string]any{"type": "integer", "minimum": 1}
			bodyProps["compat_mode"] = map[string]any{"enum": []string{"v1.4", "v1.5"}}
		case "result":
			bodyProps["epoch"] = map[string]any{"type": "integer", "minimum": 1}
		}
		if kind == "chunk" {
			bodySchema["allOf"] = []any{map[string]any{
				"if":   map[string]any{"properties": map[string]any{"stream": map[string]any{"enum": []string{"artifact", "cache"}}}, "required": []string{"stream"}},
				"then": map[string]any{"required": []string{"digest"}, "properties": map[string]any{"digest": map[string]any{"type": "string", "minLength": 1}}},
			}}
		}
		then := map[string]any{"properties": map[string]any{"body": bodySchema}}
		if leaseScoped(kind) {
			then["required"] = []string{"epoch", "lease_id"}
		}
		branches = append(branches, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"type": map[string]any{"const": kind}}, "required": []string{"type"}},
			"then": then,
		})
	}
	s := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://nself.org/schemas/runner-protocol.v1.schema.json",
		"type":    "object", "additionalProperties": false,
		"required": []string{"v", "type", "seq", "body"},
		"properties": map[string]any{
			"v": map[string]any{"const": 1}, "type": map[string]any{"enum": kinds},
			"seq":      map[string]any{"type": "integer", "minimum": 1},
			"ack":      map[string]any{"type": "integer", "minimum": 0},
			"epoch":    map[string]any{"type": "integer", "minimum": 1},
			"lease_id": map[string]any{"type": "string", "minLength": 1},
			"body":     map[string]any{"type": "object"},
		}, "allOf": branches,
	}
	b, err := json.MarshalIndent(s, "", "  ")
	return append(b, '\n'), err
}

func leaseScoped(kind string) bool {
	switch kind {
	case "lease", "result", "log", "artifact-ref", "cache-ref", "cancel":
		return true
	}
	return false
}

func typeSchema(typ reflect.Type) map[string]any {
	if typ.Kind() == reflect.Pointer {
		inner := typeSchema(typ.Elem())
		return map[string]any{"anyOf": []any{inner, map[string]any{"type": "null"}}}
	}
	switch typ.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return map[string]any{}
		}
		return map[string]any{"type": "array", "items": typeSchema(typ.Elem())}
	case reflect.Struct:
		if typ.PkgPath() != reflect.TypeOf(Hello{}).PkgPath() {
			return map[string]any{"type": "object"}
		}
		props := map[string]any{}
		required := []string{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			if tag[0] == "" || tag[0] == "-" {
				continue
			}
			props[tag[0]] = typeSchema(field.Type)
			if len(tag) == 1 || tag[1] != "omitempty" {
				required = append(required, tag[0])
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
	default:
		return map[string]any{}
	}
}
