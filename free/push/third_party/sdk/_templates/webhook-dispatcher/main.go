package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"

	sdklogger  "github.com/nself-org/cli/sdk/go/logger"
	sdkplugin  "github.com/nself-org/cli/sdk/go/plugin"
	sdkserver  "github.com/nself-org/cli/sdk/go/server"
)

const (
	pluginName    = "webhook-dispatcher"
	pluginVersion = "0.1.0"
	pluginPort    = 3901
)

func main() {
	log := sdklogger.New(pluginName, pluginVersion)
	srv := sdkserver.New(sdkserver.Config{Port: pluginPort, Logger: log})

	secret := os.Getenv("WEBHOOK_DISPATCHER_SECRET")

	srv.Handle("/webhook-dispatcher/ingest", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !verifySignature(body, r.Header.Get("X-Hub-Signature-256"), secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		// TODO: enqueue to Redis for fan-out delivery
		log.Info("inbound event received", "bytes", len(body))
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"queued": true})
	}))

	sdkplugin.Run(srv, sdkplugin.Info{
		Name:    pluginName,
		Version: pluginVersion,
		Logger:  log,
	})
}

func verifySignature(body []byte, sig, secret string) bool {
	if secret == "" {
		return true // dev mode
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}
