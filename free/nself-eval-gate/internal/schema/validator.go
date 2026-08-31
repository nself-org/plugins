package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// ValidateEvalSet validates a YAML eval-set file against the eval-set-v1.json JSON Schema.
// Purpose: Ensure eval-set YAML files are structurally correct before loading or running.
// Inputs: yamlContent — raw YAML bytes of an eval-set file.
// Outputs: []ValidationError (one per violation) on failure; nil slice on valid input.
// Constraints: Uses gojsonschema for schema enforcement. YAML is converted to JSON internally.
//
//	Called at: nself ci eval --validate-only, /eval/validate endpoint, suite load.
func ValidateEvalSet(yamlContent []byte) ([]ValidationError, error) {
	// Parse YAML to generic map first.
	var rawDoc interface{}
	if err := yaml.Unmarshal(yamlContent, &rawDoc); err != nil {
		return []ValidationError{{Field: "(root)", Message: fmt.Sprintf("YAML parse error: %v", err)}}, nil
	}

	// Convert to JSON-compatible structure (YAML maps use interface{} keys).
	jsonCompatible := convertYAMLToJSON(rawDoc)

	// Re-encode as JSON for gojsonschema.
	jsonBytes, err := json.Marshal(jsonCompatible)
	if err != nil {
		return nil, fmt.Errorf("ValidateEvalSet: JSON marshal for validation: %w", err)
	}

	// Load the JSON Schema file.
	schemaLoader, err := loadSchemaLoader()
	if err != nil {
		return nil, fmt.Errorf("ValidateEvalSet: load schema: %w", err)
	}

	docLoader := gojsonschema.NewBytesLoader(jsonBytes)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return nil, fmt.Errorf("ValidateEvalSet: schema validation error: %w", err)
	}

	if result.Valid() {
		return nil, nil
	}

	errs := make([]ValidationError, 0, len(result.Errors()))
	for _, re := range result.Errors() {
		errs = append(errs, ValidationError{
			Field:   re.Field(),
			Message: re.Description(),
		})
	}
	return errs, nil
}

// loadSchemaLoader returns a gojsonschema Loader for eval-set-v1.json.
// Tries embedded path first (relative to this file), then falls back to env var.
func loadSchemaLoader() (gojsonschema.JSONLoader, error) {
	// Try relative path from this source file.
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema", "eval-set-v1.json")
		if _, err := os.Stat(schemaPath); err == nil {
			return gojsonschema.NewReferenceLoader("file://" + schemaPath), nil
		}
	}

	// Fallback: look for NSELF_EVAL_GATE_SCHEMA_PATH env var.
	if envPath := os.Getenv("NSELF_EVAL_GATE_SCHEMA_PATH"); envPath != "" {
		return gojsonschema.NewReferenceLoader("file://" + envPath), nil
	}

	// Final fallback: embed the schema inline.
	return gojsonschema.NewStringLoader(inlineSchema), nil
}

// convertYAMLToJSON converts YAML-parsed interface{} values to JSON-compatible types.
// YAML maps use map[interface{}]interface{} keys which JSON cannot marshal.
func convertYAMLToJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			result[fmt.Sprintf("%v", k)] = convertYAMLToJSON(v2)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			result[k] = convertYAMLToJSON(v2)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v2 := range val {
			result[i] = convertYAMLToJSON(v2)
		}
		return result
	default:
		return val
	}
}

// inlineSchema is a minimal fallback schema used when the schema file cannot be located.
// This should never be used in production; it ensures graceful degradation.
const inlineSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["version", "suite", "repo", "tasks"],
  "properties": {
    "version": {"type": "string"},
    "suite": {"type": "string"},
    "repo": {"type": "string"},
    "tasks": {"type": "array", "minItems": 1, "items": {
      "type": "object",
      "required": ["id", "query", "scoring_mode", "metrics", "threshold"],
      "properties": {
        "id": {"type": "string"},
        "query": {"type": "string"},
        "scoring_mode": {"type": "string", "enum": ["exact", "semantic", "rubric"]},
        "metrics": {"type": "array"},
        "threshold": {"type": "number", "minimum": 0, "maximum": 1}
      }
    }}
  }
}`
