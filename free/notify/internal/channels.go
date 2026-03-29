package internal

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"
)

// ChannelResult holds the outcome of a single channel dispatch.
type ChannelResult struct {
	Channel string  `json:"channel"`
	Success bool    `json:"success"`
	Error   *string `json:"error,omitempty"`
}

func errPtr(s string) *string {
	return &s
}

// SendEmail sends an email via net/smtp using SMTP_* environment variables.
func SendEmail(to, subject, body string) ChannelResult {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if host == "" {
		return ChannelResult{Channel: "email", Success: false, Error: errPtr("SMTP_HOST not configured")}
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		from = "noreply@nself.org"
	}
	if to == "" {
		return ChannelResult{Channel: "email", Success: false, Error: errPtr("recipient email is empty")}
	}

	addr := host + ":" + port

	// Build RFC 2822 message.
	var msg strings.Builder
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
	if err != nil {
		return ChannelResult{Channel: "email", Success: false, Error: errPtr(fmt.Sprintf("smtp send: %v", err))}
	}

	return ChannelResult{Channel: "email", Success: true}
}

// SendWebhook sends an HTTP POST with a JSON payload to the given URL.
// If WEBHOOK_HMAC_SECRET is set, the payload is signed with HMAC-SHA256
// and the signature is included in the X-Signature-256 header.
func SendWebhook(url, payload string) ChannelResult {
	if url == "" {
		return ChannelResult{Channel: "webhook", Success: false, Error: errPtr("webhook url is empty")}
	}

	body := []byte(payload)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChannelResult{Channel: "webhook", Success: false, Error: errPtr(fmt.Sprintf("build request: %v", err))}
	}
	req.Header.Set("Content-Type", "application/json")

	// HMAC signing.
	secret := os.Getenv("WEBHOOK_HMAC_SECRET")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Signature-256", "sha256="+sig)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ChannelResult{Channel: "webhook", Success: false, Error: errPtr(fmt.Sprintf("http post: %v", err))}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ChannelResult{Channel: "webhook", Success: true}
	}

	return ChannelResult{Channel: "webhook", Success: false, Error: errPtr(fmt.Sprintf("webhook returned %d", resp.StatusCode))}
}

// buildWebhookPayload creates a standard JSON payload for webhook delivery.
func buildWebhookPayload(subject, body string) string {
	data := map[string]string{"subject": subject, "body": body}
	b, _ := json.Marshal(data)
	return string(b)
}
