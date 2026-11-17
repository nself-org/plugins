// Package envelope defines the frozen channels.v1 contract: the message
// envelope every adapter produces and consumes, the origin-trust tags it
// carries, the delivery receipt and the adapter capability set.
//
// The JSON twin of these types lives in schema/channels-v1.schema.json and
// must keep exactly the same field set.
package envelope

import (
	"encoding/json"
	"time"
)

// SchemaVersion is the version of the channels.v1 contract and its JSON Schema.
const SchemaVersion = "1.0.0"

// ChannelKind names the messaging network an adapter speaks to.
type ChannelKind string

// Channel kinds supported by channels.v1.
const (
	ChannelTelegram ChannelKind = "telegram"
	ChannelDiscord  ChannelKind = "discord"
	ChannelSlack    ChannelKind = "slack"
	ChannelTeams    ChannelKind = "teams"
	ChannelMatrix   ChannelKind = "matrix"
	ChannelWhatsApp ChannelKind = "whatsapp"
	ChannelEmail    ChannelKind = "email"
	ChannelSMS      ChannelKind = "sms"
	ChannelWebchat  ChannelKind = "webchat"
)

// ChannelKinds lists every channel kind in schema order.
var ChannelKinds = []ChannelKind{
	ChannelTelegram, ChannelDiscord, ChannelSlack, ChannelTeams, ChannelMatrix,
	ChannelWhatsApp, ChannelEmail, ChannelSMS, ChannelWebchat,
}

// Direction says whether an envelope arrived from a provider or is leaving for one.
type Direction string

// Envelope directions.
const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

// Directions lists every direction in schema order.
var Directions = []Direction{DirectionInbound, DirectionOutbound}

// PartKind names the type of one ordered message part.
type PartKind string

// Part kinds.
const (
	PartText         PartKind = "text"
	PartApprovalCard PartKind = "approval_card"
	PartAction       PartKind = "action"
	PartAttachment   PartKind = "attachment"
)

// PartKinds lists every part kind in schema order.
var PartKinds = []PartKind{PartText, PartApprovalCard, PartAction, PartAttachment}

// TrustTag is one origin-trust fact about where a message came from.
//
// The set is open: adapters and upstream stages may add tags not listed
// here, and every consumer must carry unknown tags through unchanged.
// Tags are carried, never judged. Nothing in this package scores, ranks or
// filters an envelope by its tags; policy lives elsewhere.
type TrustTag string

// Known origin-trust tags. The list documents common facts; it is not a closed enum.
const (
	TrustPairedSender    TrustTag = "paired_sender"
	TrustAllowlisted     TrustTag = "allowlisted"
	TrustUnknownSender   TrustTag = "unknown_sender"
	TrustGroupMessage    TrustTag = "group_message"
	TrustForwarded       TrustTag = "forwarded"
	TrustExternalContent TrustTag = "external_content"
	TrustBotSender       TrustTag = "bot_sender"
)

// KnownTrustTags lists the documented origin-trust tags in schema order.
var KnownTrustTags = []TrustTag{
	TrustPairedSender, TrustAllowlisted, TrustUnknownSender, TrustGroupMessage,
	TrustForwarded, TrustExternalContent, TrustBotSender,
}

// Envelope is one message crossing a channel boundary, in either direction.
type Envelope struct {
	EnvelopeID        string       `json:"envelope_id"`
	TenantID          string       `json:"tenant_id"`
	AdapterInstance   string       `json:"adapter_instance"`
	Channel           ChannelKind  `json:"channel"`
	Direction         Direction    `json:"direction"`
	SenderExternalID  string       `json:"sender_external_id,omitempty"`
	ConversationRef   string       `json:"conversation_ref"`
	ExternalThreadRef string       `json:"external_thread_ref,omitempty"`
	ParentID          string       `json:"parent_id,omitempty"`
	EditOf            string       `json:"edit_of,omitempty"`
	Parts             []Part       `json:"parts"`
	Attachments       []Attachment `json:"attachments,omitempty"`
	MentionsBot       bool         `json:"mentions_bot"`
	OriginTrust       []TrustTag   `json:"origin_trust"`
	ReceivedAt        time.Time    `json:"received_at"`
	// ProviderMeta holds provider-specific data opaque to every other stage.
	ProviderMeta map[string]json.RawMessage `json:"provider_meta,omitempty"`
}

// Part is one ordered piece of message content.
type Part struct {
	Kind PartKind `json:"kind"`
	// Text is the body of a text part or the prompt of an approval card.
	Text string `json:"text,omitempty"`
	// Title heads an approval card.
	Title string `json:"title,omitempty"`
	// Actions are the buttons offered by an approval card.
	Actions []Action `json:"actions,omitempty"`
	// Action is the button a user pressed, for an action part.
	Action *Action `json:"action,omitempty"`
	// AttachmentDigest points at an entry in Envelope.Attachments.
	AttachmentDigest string `json:"attachment_digest,omitempty"`
}

// Action is one button on an approval card, or the button a user pressed.
type Action struct {
	ActionID string `json:"action_id"`
	Label    string `json:"label,omitempty"`
	Value    string `json:"value,omitempty"`
}

// Attachment references a blob by content digest; bytes never ride in the envelope.
type Attachment struct {
	Digest    string `json:"digest"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Filename  string `json:"filename,omitempty"`
}

// ReceiptState is the delivery state of an outbound envelope.
type ReceiptState string

// Receipt states.
const (
	ReceiptQueued    ReceiptState = "queued"
	ReceiptSent      ReceiptState = "sent"
	ReceiptDelivered ReceiptState = "delivered"
	ReceiptRead      ReceiptState = "read"
	ReceiptFailed    ReceiptState = "failed"
	ReceiptDead      ReceiptState = "dead"
)

// ReceiptStates lists every receipt state in schema order.
var ReceiptStates = []ReceiptState{
	ReceiptQueued, ReceiptSent, ReceiptDelivered, ReceiptRead, ReceiptFailed, ReceiptDead,
}

// Receipt records the delivery progress of one outbound envelope.
type Receipt struct {
	EnvelopeID        string       `json:"envelope_id"`
	State             ReceiptState `json:"state"`
	Attempts          int          `json:"attempts"`
	ProviderMessageID string       `json:"provider_message_id,omitempty"`
	LastError         string       `json:"last_error,omitempty"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// Capabilities describes what an adapter's provider supports.
type Capabilities struct {
	// MaxText is the longest text body in characters; 0 means no known limit.
	MaxText     int  `json:"max_text"`
	Edits       bool `json:"edits"`
	Threads     bool `json:"threads"`
	Buttons     bool `json:"buttons"`
	Attachments bool `json:"attachments"`
	Typing      bool `json:"typing"`
}
