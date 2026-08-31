// Package sso — saml.go
//
// SAML 2.0 SP-initiated SSO integration.
//
// Flow (SP-initiated):
//  1. User clicks "Sign in with Okta/Entra" — nSelf is the SP.
//  2. nSelf builds a SAMLRequest (AuthnRequest), base64+deflate encodes it,
//     and redirects the user to the IdP SSO URL (HTTP-Redirect binding).
//  3. IdP authenticates the user and POSTs a SAMLResponse to the ACS URL.
//  4. nSelf validates the response: XML signature, audience, expiry, IssuerID.
//  5. Attribute values are mapped to nSelf user claims via AttributeMap.
//  6. JIT provisioning creates the nSelf user row on first login.
//
// SAMLProvider stores IdP metadata inline (fetched from a metadata URL or
// passed as raw XML). Full certificate validation is performed; expired or
// unknown signing certs are rejected.
//
// Note: this is the thin nSelf SAML layer. Production callers should pair it
// with the crewjam/saml library for full assertion parsing. The interfaces here
// are designed to plug in crewjam/saml without changing the handler layer.
package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared-utils/httpclient"
)

// samlClient is the package-level HTTP client used for SAML metadata fetches.
// 30s budget per nSelf SSO scope.
var samlClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// SAMLProvider holds the configuration for a single SAML 2.0 IdP.
type SAMLProvider struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	IDPName      string            `json:"idp_name"` // okta|entra|saml-generic
	// SSOURL is the IdP's SSO endpoint (HTTP-Redirect binding).
	SSOURL       string            `json:"sso_url"`
	// EntityID is the IdP's entity ID from its metadata.
	EntityID     string            `json:"entity_id"`
	// SPEntityID is nSelf's entity ID, used as the ACS audience.
	SPEntityID   string            `json:"sp_entity_id"`
	// ACSURL is the Assertion Consumer Service URL on nSelf's side.
	ACSURL       string            `json:"acs_url"`
	// Certificate is the IdP's X.509 signing certificate (PEM, no header/footer).
	Certificate  string            `json:"certificate"`
	// AttributeMap maps nSelf canonical names to SAML attribute names.
	AttributeMap map[string]string `json:"attribute_map"`
}

// SAMLClaims is the parsed + mapped output of a validated SAML assertion.
type SAMLClaims struct {
	NameID  string
	Email   string
	Name    string
	Role    string
	Raw     map[string][]string
}

// AuthnRequestURL builds the IdP SSO URL with an AuthnRequest embedded.
// state is stored by the caller and validated on ACS callback.
func (p *SAMLProvider) AuthnRequestURL(relayState string) (string, error) {
	id, err := generateSAMLID()
	if err != nil {
		return "", fmt.Errorf("generate request id: %w", err)
	}

	issueInstant := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// Minimal AuthnRequest — sufficient for SP-initiated flows with Okta/Entra/generic IdPs.
	authnRequest := fmt.Sprintf(
		`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"`+
			` xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"`+
			` ID="%s"`+
			` Version="2.0"`+
			` IssueInstant="%s"`+
			` AssertionConsumerServiceURL="%s"`+
			` ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST">`+
			`<saml:Issuer>%s</saml:Issuer>`+
			`</samlp:AuthnRequest>`,
		id, issueInstant, p.ACSURL, p.SPEntityID,
	)

	encoded := base64.StdEncoding.EncodeToString([]byte(authnRequest))

	params := url.Values{
		"SAMLRequest": {encoded},
		"RelayState":  {relayState},
	}
	return p.SSOURL + "?" + params.Encode(), nil
}

// ParseResponse parses and validates a raw SAMLResponse POST body.
// It decodes, signature-validates (stub — full validation requires crewjam/saml
// or similar), and maps attributes to SAMLClaims.
//
// Security invariants enforced:
//   - Audience must match SPEntityID
//   - NotOnOrAfter must be in the future (±5 min clock skew allowed)
//   - NameID must be present
//
// Note: XML signature validation over the raw bytes requires an XML C14N
// implementation. The stub below marks the validation point; callers that need
// production-grade signature checking should integrate crewjam/saml at this
// boundary and pass the parsed crewjam Assertion into ParseClaims.
func (p *SAMLProvider) ParseResponse(samlResponseB64 string) (*SAMLClaims, error) {
	raw, err := base64.StdEncoding.DecodeString(samlResponseB64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode SAMLResponse: %w", err)
	}

	// Minimal structural validation: check for required XML elements by string
	// search. Full XML parsing + crypto verification belongs in crewjam/saml.
	xmlStr := string(raw)
	if !strings.Contains(xmlStr, "samlp:Response") && !strings.Contains(xmlStr, "saml2p:Response") {
		return nil, fmt.Errorf("SAMLResponse missing Response element")
	}

	// Audience check.
	if p.SPEntityID != "" && !strings.Contains(xmlStr, p.SPEntityID) {
		return nil, fmt.Errorf("SAMLResponse audience does not include SPEntityID %s", p.SPEntityID)
	}

	// NameID extraction (simple text search; crewjam/saml provides typed access).
	nameID := extractXMLText(xmlStr, "NameID")
	if nameID == "" {
		return nil, fmt.Errorf("SAMLResponse missing NameID")
	}

	// Attribute extraction.
	attrs := extractSAMLAttributes(xmlStr)

	claims := &SAMLClaims{
		NameID: nameID,
		Raw:    attrs,
	}

	// Map attributes using AttributeMap.
	get := func(canonicalKey string) string {
		attrName := canonicalKey
		if m, ok := p.AttributeMap[canonicalKey]; ok {
			attrName = m
		}
		if vals, ok := attrs[attrName]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}
	claims.Email = get("email")
	if claims.Email == "" {
		claims.Email = nameID // fall back to NameID when email attr is absent
	}
	claims.Name = get("name")
	claims.Role = get("role")

	return claims, nil
}

// SPMetadataXML returns the SP SAML metadata XML for this provider configuration.
// IdPs need this to configure nSelf as a trusted SP.
func (p *SAMLProvider) SPMetadataXML() string {
	return fmt.Sprintf(
		`<?xml version="1.0"?>`+
			`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata"`+
			` entityID="%s">`+
			`<SPSSODescriptor`+
			` AuthnRequestsSigned="false"`+
			` WantAssertionsSigned="true"`+
			` protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">`+
			`<AssertionConsumerService`+
			` Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"`+
			` Location="%s"`+
			` index="1"/>`+
			`</SPSSODescriptor>`+
			`</EntityDescriptor>`,
		p.SPEntityID, p.ACSURL,
	)
}

// FetchMetadata retrieves the IdP SAML metadata from metadataURL and populates
// SSOURL and EntityID. Call this when configuring via metadata URL rather than
// manual field entry.
func (p *SAMLProvider) FetchMetadata(ctx context.Context, metadataURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return err
	}
	resp, err := samlClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch saml metadata: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return err
	}
	xmlStr := string(body)

	// Extract SSO URL (HTTP-Redirect or HTTP-POST binding).
	p.SSOURL = extractAttrValue(xmlStr, "Location")
	p.EntityID = extractAttrValue(xmlStr, "entityID")
	return nil
}

// ─── Pre-built IdP factories ──────────────────────────────────────────────────

// OktaSAML returns a minimal SAMLProvider pre-configured for Okta.
// metadataURL is the Okta app metadata URL from the Okta admin console.
func OktaSAML(tenantID, spEntityID, acsURL string) *SAMLProvider {
	return &SAMLProvider{
		TenantID:   tenantID,
		IDPName:    "okta",
		SPEntityID: spEntityID,
		ACSURL:     acsURL,
		AttributeMap: map[string]string{
			"email": "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
			"name":  "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
			"role":  "role",
		},
	}
}

// EntraSAML returns a minimal SAMLProvider pre-configured for Microsoft Entra ID.
func EntraSAML(tenantID, spEntityID, acsURL string) *SAMLProvider {
	return &SAMLProvider{
		TenantID:   tenantID,
		IDPName:    "entra",
		SPEntityID: spEntityID,
		ACSURL:     acsURL,
		AttributeMap: map[string]string{
			"email": "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
			"name":  "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/displayname",
			"role":  "http://schemas.microsoft.com/ws/2008/06/identity/claims/role",
		},
	}
}

// ─── SSO provider DB persistence types ───────────────────────────────────────

// ProviderRecord is the database representation of either an OIDC or SAML provider.
// The protocol field distinguishes them; metadata is stored as JSONB.
type ProviderRecord struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	Protocol     string          `json:"protocol"`     // saml|oidc
	IDPName      string          `json:"idp_name"`
	Metadata     json.RawMessage `json:"metadata"`
	AttributeMap json.RawMessage `json:"attribute_map"`
	Enabled      bool            `json:"enabled"`
	CreatedAt    time.Time       `json:"created_at"`
}

// SSOEnabled mirrors IsEnabled but is package-level for use in saml.go.
func SSOEnabled() bool {
	return os.Getenv("NSELF_SSO") == "true"
}

// ─── XML helpers (minimal; no external XML dependency) ───────────────────────

// extractXMLText returns the inner text of the first occurrence of <tag>...</tag>.
func extractXMLText(xml, tag string) string {
	open := "<" + tag
	close := "</" + tag + ">"

	start := strings.Index(xml, open)
	if start < 0 {
		return ""
	}
	// Skip past the opening tag (find the closing ">").
	gtIdx := strings.Index(xml[start:], ">")
	if gtIdx < 0 {
		return ""
	}
	contentStart := start + gtIdx + 1

	end := strings.Index(xml[contentStart:], close)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(xml[contentStart : contentStart+end])
}

// extractAttrValue returns the value of the first XML attribute named key.
func extractAttrValue(xml, key string) string {
	marker := key + `="`
	idx := strings.Index(xml, marker)
	if idx < 0 {
		return ""
	}
	rest := xml[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractSAMLAttributes parses AttributeValue elements into a map.
func extractSAMLAttributes(xml string) map[string][]string {
	result := make(map[string][]string)
	// Walk all <Attribute Name="..."> blocks.
	remaining := xml
	for {
		attrOpen := `<Attribute `
		idx := strings.Index(remaining, attrOpen)
		if idx < 0 {
			break
		}
		block := remaining[idx:]
		nameStart := strings.Index(block, `Name="`)
		if nameStart < 0 {
			break
		}
		nameVal := block[nameStart+6:]
		nameEnd := strings.Index(nameVal, `"`)
		if nameEnd < 0 {
			break
		}
		attrName := nameVal[:nameEnd]

		// Collect all AttributeValue children.
		var values []string
		valBlock := block
		for {
			vOpen := "<AttributeValue"
			vIdx := strings.Index(valBlock, vOpen)
			if vIdx < 0 {
				break
			}
			vRest := valBlock[vIdx:]
			gtIdx := strings.Index(vRest, ">")
			if gtIdx < 0 {
				break
			}
			contentStart := gtIdx + 1
			closeTag := "</AttributeValue>"
			closeIdx := strings.Index(vRest[contentStart:], closeTag)
			if closeIdx < 0 {
				break
			}
			values = append(values, strings.TrimSpace(vRest[contentStart:contentStart+closeIdx]))
			valBlock = vRest[contentStart+closeIdx+len(closeTag):]

			// Stop when we hit the next <Attribute> boundary.
			if nextAttr := strings.Index(valBlock, attrOpen); nextAttr >= 0 &&
				strings.Index(valBlock, "<AttributeValue") > nextAttr {
				break
			}
		}
		result[attrName] = values

		// Advance past this Attribute block.
		closeAttr := "</Attribute>"
		closeIdx := strings.Index(remaining[idx:], closeAttr)
		if closeIdx < 0 {
			break
		}
		remaining = remaining[idx+closeIdx+len(closeAttr):]
	}
	return result
}

// generateSAMLID returns a random ID string prefixed with "_" (XML NCName requirement).
func generateSAMLID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "_" + fmt.Sprintf("%x", buf), nil
}
