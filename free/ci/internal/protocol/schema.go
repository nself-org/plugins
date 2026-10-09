package protocol

import "encoding/json"

// Schema returns deterministic JSON Schema for the v1 frame and message set.
func Schema() ([]byte, error) {
	str := map[string]any{"type": "string"}
	uint := map[string]any{"type": "integer", "minimum": 0}
	body := map[string]any{"type": "object"}
	props := map[string]any{"v": map[string]any{"const": 1}, "type": map[string]any{"enum": []string{"hello", "welcome", "reject", "heartbeat", "lease", "secrets", "chunk", "log", "artifact-ref", "result", "cancel", "drain", "upgrade", "ack", "digest", "offset", "resume", "cache-ref"}}, "seq": map[string]any{"type": "integer", "minimum": 1}, "ack": uint, "epoch": uint, "lease_id": str, "body": body}
	hello := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"versions", "agent_version", "age_recipient", "node_key", "capability_digest", "identity", "compat_mode_supported", "slots"}, "properties": map[string]any{"versions": map[string]any{"type": "array", "items": uint}, "agent_version": str, "age_recipient": map[string]any{"type": []string{"string", "null"}}, "node_key": map[string]any{"type": "object"}, "capability_digest": str, "identity": map[string]any{"type": "object"}, "compat_mode_supported": map[string]any{"type": "array", "items": map[string]any{"enum": []string{"v1.4", "v1.5"}}}, "slots": map[string]any{"type": "object"}}}
	s := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://nself.org/schemas/runner-protocol.v1.schema.json", "type": "object", "additionalProperties": false, "required": []string{"v", "type", "seq", "body"}, "properties": props, "allOf": []any{map[string]any{"if": map[string]any{"properties": map[string]any{"type": map[string]any{"const": "hello"}}}, "then": map[string]any{"properties": map[string]any{"body": hello}}}}}
	b, err := json.MarshalIndent(s, "", "  ")
	return append(b, '\n'), err
}
