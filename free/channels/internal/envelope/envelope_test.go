package envelope

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

type schemaProp struct {
	Enum []string `json:"enum"`
}

type schemaDef struct {
	Enum       []string              `json:"enum"`
	Examples   []string              `json:"examples"`
	Properties map[string]schemaProp `json:"properties"`
}

type schemaDoc struct {
	Version string               `json:"version"`
	Defs    map[string]schemaDef `json:"$defs"`
}

func loadSchema(t *testing.T) schemaDoc {
	t.Helper()
	raw, err := os.ReadFile("../../schema/channels-v1.schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc schemaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return doc
}

func jsonFields(t reflect.Type) []string {
	var out []string
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func TestEnvelopeSchemaParity(t *testing.T) {
	doc := loadSchema(t)
	if doc.Version != SchemaVersion {
		t.Errorf("schema version %q, Go %q", doc.Version, SchemaVersion)
	}
	types := map[string]reflect.Type{
		"Envelope":     reflect.TypeFor[Envelope](),
		"Part":         reflect.TypeFor[Part](),
		"Action":       reflect.TypeFor[Action](),
		"Attachment":   reflect.TypeFor[Attachment](),
		"Receipt":      reflect.TypeFor[Receipt](),
		"Capabilities": reflect.TypeFor[Capabilities](),
	}
	for name, typ := range types {
		def, ok := doc.Defs[name]
		if !ok {
			t.Errorf("schema missing $defs/%s", name)
			continue
		}
		var props []string
		for k := range def.Properties {
			props = append(props, k)
		}
		sort.Strings(props)
		if got := jsonFields(typ); !slices.Equal(got, props) {
			t.Errorf("%s: Go fields %v, schema fields %v", name, got, props)
		}
	}
	for name, def := range doc.Defs {
		if _, ok := types[name]; !ok && def.Properties != nil {
			t.Errorf("schema object $defs/%s has no Go struct", name)
		}
	}
	enums := []struct {
		name   string
		schema []string
		goVals []string
	}{
		{"channel", doc.Defs["ChannelKind"].Enum, strs(ChannelKinds)},
		{"direction", doc.Defs["Envelope"].Properties["direction"].Enum, strs(Directions)},
		{"part kind", doc.Defs["Part"].Properties["kind"].Enum, strs(PartKinds)},
		{"receipt state", doc.Defs["Receipt"].Properties["state"].Enum, strs(ReceiptStates)},
		{"trust tags", doc.Defs["TrustTag"].Examples, strs(KnownTrustTags)},
	}
	for _, e := range enums {
		if !slices.Equal(e.schema, e.goVals) {
			t.Errorf("%s: schema %v, Go %v", e.name, e.schema, e.goVals)
		}
	}
	if doc.Defs["TrustTag"].Enum != nil {
		t.Error("TrustTag must stay an open set, not a closed enum")
	}
}

func sampleEnvelope() Envelope {
	return Envelope{
		EnvelopeID:        "env-1",
		TenantID:          "tenant-1",
		AdapterInstance:   "telegram-main",
		Channel:           ChannelTelegram,
		Direction:         DirectionInbound,
		SenderExternalID:  "u-42",
		ConversationRef:   "chat-7",
		ExternalThreadRef: "thread-3",
		ParentID:          "env-0",
		Parts: []Part{
			{Kind: PartText, Text: "first"},
			{Kind: PartApprovalCard, Title: "Deploy?", Text: "approve it",
				Actions: []Action{{ActionID: "yes", Label: "Yes"}, {ActionID: "no", Label: "No"}}},
			{Kind: PartAttachment, AttachmentDigest: "sha256:abcd"},
			{Kind: PartText, Text: "last"},
		},
		Attachments: []Attachment{{Digest: "sha256:abcd", MediaType: "image/png", SizeBytes: 10}},
		MentionsBot: true,
		OriginTrust: []TrustTag{TrustGroupMessage, TrustForwarded, TrustPairedSender, TrustExternalContent},
		ReceivedAt:  time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC),
		ProviderMeta: map[string]json.RawMessage{
			"update_id": json.RawMessage(`12345`),
		},
	}
}

func roundTrip(t *testing.T, in Envelope) Envelope {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Envelope
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestEnvelopeRoundTrip(t *testing.T) {
	in := sampleEnvelope()
	out := roundTrip(t, in)
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed envelope:\n in  %+v\n out %+v", in, out)
	}
	if !slices.Equal(in.OriginTrust, out.OriginTrust) {
		t.Errorf("origin_trust order: %v -> %v", in.OriginTrust, out.OriginTrust)
	}
	for i := range in.Parts {
		if in.Parts[i].Kind != out.Parts[i].Kind || in.Parts[i].Text != out.Parts[i].Text {
			t.Errorf("part %d changed: %+v -> %+v", i, in.Parts[i], out.Parts[i])
		}
	}
}

func TestEnvelopeTrustTagsOpenSet(t *testing.T) {
	in := sampleEnvelope()
	in.OriginTrust = []TrustTag{TrustUnknownSender, "custom_relay_hop", TrustBotSender}
	out := roundTrip(t, in)
	if !slices.Equal(in.OriginTrust, out.OriginTrust) {
		t.Fatalf("unknown tag not carried: %v -> %v", in.OriginTrust, out.OriginTrust)
	}
	var raw map[string]any
	b, _ := json.Marshal(in)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	tags, _ := raw["origin_trust"].([]any)
	if len(tags) != 3 || tags[1] != "custom_relay_hop" {
		t.Fatalf("wire origin_trust = %v", raw["origin_trust"])
	}
}
