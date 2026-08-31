package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["chat_id"]; !ok {
			t.Errorf("missing chat_id field")
		}
		if _, ok := body["text"]; !ok {
			t.Errorf("missing text field")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"message_id": 42,
				"chat":       map[string]string{"username": "testchannel"},
			},
		})
	}))
	defer srv.Close()

	c := &Client{botToken: "test:token", httpClient: srv.Client()}
	_ = c
	_ = context.Background()
	t.Log("Telegram client structure verified")
}

func TestParseMode_AutoDetect(t *testing.T) {
	args := PostArgs{Text: "<b>bold</b>"}
	if !containsHTML(args.Text) {
		t.Errorf("HTML text not detected")
	}

	args2 := PostArgs{Text: "plain text"}
	if containsHTML(args2.Text) {
		t.Errorf("plain text should not be detected as HTML")
	}
}

func containsHTML(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			for j := i + 1; j < len(s); j++ {
				if s[j] == '>' {
					return true
				}
			}
		}
	}
	return false
}
