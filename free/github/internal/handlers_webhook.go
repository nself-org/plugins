package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// --- Webhook endpoint --------------------------------------------------------

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Hub-Signature-256")
	event := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	if event == "" || deliveryID == "" {
		log.Printf("[github:server] Missing GitHub event headers")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing event headers"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read body"})
		return
	}
	defer r.Body.Close()

	if s.webhookSecret != "" && signature != "" {
		if !VerifySignature(body, signature, s.webhookSecret) {
			log.Printf("[github:server] Invalid GitHub signature")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid signature"})
			return
		}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	ctx := r.Context()
	if err := s.webhookHandler.Handle(ctx, deliveryID, event, payload); err != nil {
		log.Printf("[github:server] Webhook processing failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Processing failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"received": true})
}

