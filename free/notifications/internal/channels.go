package internal

import (
	"fmt"
	"log"
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

// SendPush is a placeholder for push notification delivery.
// Integrate with FCM, OneSignal, or Web Push by implementing the provider
// logic here and reading credentials from environment variables.
func SendPush(deviceToken, title, body string) ChannelResult {
	provider := os.Getenv("NOTIFICATIONS_PUSH_PROVIDER")
	if provider == "" {
		return ChannelResult{Channel: "push", Success: false, Error: errPtr("NOTIFICATIONS_PUSH_PROVIDER not configured")}
	}

	// Placeholder: log the push attempt. Replace with real provider SDK calls.
	log.Printf("[nself-notifications] push(%s): token=%s title=%q", provider, deviceToken, title)

	return ChannelResult{Channel: "push", Success: false, Error: errPtr("push provider not implemented: " + provider)}
}

// SendSMS is a placeholder for SMS delivery.
// Integrate with Twilio, Plivo, or AWS SNS by implementing the provider
// logic here and reading credentials from environment variables.
func SendSMS(phoneNumber, body string) ChannelResult {
	provider := os.Getenv("NOTIFICATIONS_SMS_PROVIDER")
	if provider == "" {
		return ChannelResult{Channel: "sms", Success: false, Error: errPtr("NOTIFICATIONS_SMS_PROVIDER not configured")}
	}

	if phoneNumber == "" {
		return ChannelResult{Channel: "sms", Success: false, Error: errPtr("recipient phone number is empty")}
	}

	// Placeholder: log the SMS attempt. Replace with real provider SDK calls.
	log.Printf("[nself-notifications] sms(%s): to=%s", provider, phoneNumber)

	return ChannelResult{Channel: "sms", Success: false, Error: errPtr("sms provider not implemented: " + provider)}
}
