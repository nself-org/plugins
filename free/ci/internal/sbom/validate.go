package sbom

// Purpose: the run-time structural check of a generated SBOM (Epic CACHE D13).
// Inputs:  the CycloneDX or SPDX JSON bytes, the artifact name, the syft pin.
// Outputs: nil, or E718 invalid naming the first thing wrong.
// Constraints: structural only (the full JSON Schema is checked in tests with
// the vendored schema); CycloneDX must be bomFormat CycloneDX, specVersion 1.6,
// a urn:uuid serialNumber, metadata.tools naming syft at the pinned version,
// metadata.component.name equal to the artifact name, and components, when
// present, an array (syft omits the key for an artifact with no packages);
// SPDX must be SPDX-2.3 with its mandatory document fields.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

var uuidURN = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type cdxDoc struct {
	BomFormat    string          `json:"bomFormat"`
	SpecVersion  string          `json:"specVersion"`
	SerialNumber string          `json:"serialNumber"`
	Version      *int            `json:"version"`
	Metadata     *cdxMetadata    `json:"metadata"`
	Components   json.RawMessage `json:"components"`
}

type cdxMetadata struct {
	Tools     json.RawMessage `json:"tools"`
	Component *struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"component"`
}

type cdxTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ValidateCycloneDX checks doc is a CycloneDX 1.6 SBOM of artifact name made by syft at version.
func ValidateCycloneDX(doc []byte, name, syftVersion string) error {
	_, err := checkCycloneDX(doc, name, syftVersion)
	return err
}

// checkCycloneDX is ValidateCycloneDX that also returns the component count.
func checkCycloneDX(doc []byte, name, syftVersion string) (int, error) {
	var d cdxDoc
	if err := strictJSON(doc, &d); err != nil {
		return 0, Failf(ReasonInvalid, "CycloneDX document: %v", err)
	}
	switch {
	case d.BomFormat != "CycloneDX":
		return 0, Failf(ReasonInvalid, "bomFormat is %q, want CycloneDX", d.BomFormat)
	case d.SpecVersion != "1.6":
		return 0, Failf(ReasonInvalid, "specVersion is %q, want 1.6", d.SpecVersion)
	case !uuidURN.MatchString(d.SerialNumber):
		return 0, Failf(ReasonInvalid, "serialNumber %q is not a urn:uuid", d.SerialNumber)
	case d.Version == nil || *d.Version < 1:
		return 0, Failf(ReasonInvalid, "version is missing or below 1")
	case d.Metadata == nil:
		return 0, Failf(ReasonInvalid, "metadata is missing")
	}
	if !namesTool(d.Metadata.Tools, syftVersion) {
		return 0, Failf(ReasonInvalid, "metadata.tools does not name %s %s", ToolName, syftVersion)
	}
	if d.Metadata.Component != nil && d.Metadata.Component.Name == name && d.Metadata.Component.Type == "" {
		return 0, Failf(ReasonInvalid, "metadata.component.type is missing")
	}
	if d.Metadata.Component == nil || d.Metadata.Component.Name != name {
		got := ""
		if d.Metadata.Component != nil {
			got = d.Metadata.Component.Name
		}
		return 0, Failf(ReasonInvalid, "metadata.component.name is %q, want the artifact name %q", got, name)
	}
	return checkComponents(d.Components)
}

// checkComponents requires components, when present, to be an array of objects
// that each carry a type and a name.
func checkComponents(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || arr == nil {
		return 0, Failf(ReasonInvalid, "components is not an array")
	}
	for i, el := range arr {
		var c struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if !bytes.HasPrefix(bytes.TrimSpace(el), []byte("{")) || json.Unmarshal(el, &c) != nil || c.Type == "" || c.Name == "" {
			return 0, Failf(ReasonInvalid, "components[%d] is not an object with a type and a name", i)
		}
	}
	return len(arr), nil
}

// namesTool accepts both CycloneDX 1.5+ tools shapes: {components:[...]} and the legacy [...] list.
func namesTool(raw json.RawMessage, version string) bool {
	var tools []cdxTool
	var obj struct {
		Components []cdxTool `json:"components"`
		Services   []cdxTool `json:"services"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		tools = append(obj.Components, obj.Services...)
	} else if json.Unmarshal(raw, &tools) != nil {
		return false
	}
	for _, t := range tools {
		if strings.EqualFold(t.Name, ToolName) && t.Version == version {
			return true
		}
	}
	return false
}

type spdxDoc struct {
	SPDXVersion       string `json:"spdxVersion"`
	DataLicense       string `json:"dataLicense"`
	SPDXID            string `json:"SPDXID"`
	Name              string `json:"name"`
	DocumentNamespace string `json:"documentNamespace"`
	CreationInfo      *struct {
		Creators []string `json:"creators"`
	} `json:"creationInfo"`
	Packages []json.RawMessage `json:"packages"`
}

// ValidateSPDX checks doc is an SPDX 2.3 JSON document of artifact name made by syft at version.
func ValidateSPDX(doc []byte, name, syftVersion string) error {
	var d spdxDoc
	if err := strictJSON(doc, &d); err != nil {
		return Failf(ReasonInvalid, "SPDX document: %v", err)
	}
	switch {
	case d.SPDXVersion != "SPDX-2.3":
		return Failf(ReasonInvalid, "spdxVersion is %q, want SPDX-2.3", d.SPDXVersion)
	case d.DataLicense != "CC0-1.0":
		return Failf(ReasonInvalid, "dataLicense is %q, want CC0-1.0", d.DataLicense)
	case d.SPDXID != "SPDXRef-DOCUMENT":
		return Failf(ReasonInvalid, "SPDXID is %q, want SPDXRef-DOCUMENT", d.SPDXID)
	case d.Name == "" || d.DocumentNamespace == "" || d.CreationInfo == nil:
		return Failf(ReasonInvalid, "name, documentNamespace or creationInfo is missing")
	case d.Name != name:
		return Failf(ReasonInvalid, "SPDX name is %q, want the artifact name %q", d.Name, name)
	}
	for _, c := range d.CreationInfo.Creators {
		if strings.EqualFold(c, "Tool: "+ToolName+"-"+syftVersion) {
			return nil
		}
	}
	return Failf(ReasonInvalid, "SPDX creationInfo does not name Tool: %s-%s", ToolName, syftVersion)
}

// strictJSON decodes exactly one JSON value; any trailing bytes (a stray
// closing brace included) are refused.
func strictJSON(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errTrailing
	}
	return nil
}

var errTrailing = Failf(ReasonInvalid, "trailing data after the JSON value")
