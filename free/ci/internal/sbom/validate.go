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
		Name string `json:"name"`
	} `json:"component"`
}

type cdxTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ValidateCycloneDX checks doc is a CycloneDX 1.6 SBOM of artifact name made by syft at version.
func ValidateCycloneDX(doc []byte, name, syftVersion string) error {
	var d cdxDoc
	if err := strictJSON(doc, &d); err != nil {
		return Failf(ReasonInvalid, "CycloneDX document: %v", err)
	}
	switch {
	case d.BomFormat != "CycloneDX":
		return Failf(ReasonInvalid, "bomFormat is %q, want CycloneDX", d.BomFormat)
	case d.SpecVersion != "1.6":
		return Failf(ReasonInvalid, "specVersion is %q, want 1.6", d.SpecVersion)
	case !uuidURN.MatchString(d.SerialNumber):
		return Failf(ReasonInvalid, "serialNumber %q is not a urn:uuid", d.SerialNumber)
	case d.Version == nil || *d.Version < 1:
		return Failf(ReasonInvalid, "version is missing or below 1")
	case d.Metadata == nil:
		return Failf(ReasonInvalid, "metadata is missing")
	}
	if !namesTool(d.Metadata.Tools, syftVersion) {
		return Failf(ReasonInvalid, "metadata.tools does not name %s %s", ToolName, syftVersion)
	}
	if d.Metadata.Component == nil || d.Metadata.Component.Name != name {
		got := ""
		if d.Metadata.Component != nil {
			got = d.Metadata.Component.Name
		}
		return Failf(ReasonInvalid, "metadata.component.name is %q, want the artifact name %q", got, name)
	}
	if len(d.Components) > 0 {
		var arr []json.RawMessage
		if err := json.Unmarshal(d.Components, &arr); err != nil || bytes.Equal(bytes.TrimSpace(d.Components), []byte("null")) {
			return Failf(ReasonInvalid, "components is not an array")
		}
	}
	return nil
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
	SPDXVersion       string            `json:"spdxVersion"`
	DataLicense       string            `json:"dataLicense"`
	SPDXID            string            `json:"SPDXID"`
	Name              string            `json:"name"`
	DocumentNamespace string            `json:"documentNamespace"`
	CreationInfo      *struct{}         `json:"creationInfo"`
	Packages          []json.RawMessage `json:"packages"`
}

// ValidateSPDX checks doc parses as an SPDX 2.3 JSON document.
func ValidateSPDX(doc []byte) error {
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
	}
	return nil
}

// strictJSON decodes one JSON value and refuses trailing data.
func strictJSON(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errTrailing
	}
	return nil
}

var errTrailing = Failf(ReasonInvalid, "trailing data after the JSON value")
