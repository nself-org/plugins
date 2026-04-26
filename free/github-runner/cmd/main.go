package main

import (
	"log"
	"os"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-github-runner/internal"
)

func main() {
	cfg := sdk.LoadConfig()

	// Default port 3054 unless PORT env overrides it.
	// Port 3053 belongs to the push plugin (APNs/FCM relay).
	port := 3054
	if cfg.Port != 3000 {
		port = cfg.Port
	}

	runnerDir := os.Getenv("GITHUB_RUNNER_DIR")
	if runnerDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("cannot determine home directory: %v", err)
		}
		runnerDir = home + "/.nself/runners/github-runner"
	}

	state := internal.NewRunnerState()
	h := internal.NewHandler(state)

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)
	r.Use(sdk.Logger)

	// Override the SDK default /health with our runner-aware version.
	r.Get("/health", h.Health)

	r.Get("/v1/status", h.Status)
	r.Post("/v1/register", h.Register)
	r.Post("/v1/start", h.Start)
	r.Post("/v1/stop", h.Stop)
	r.Get("/v1/logs", h.Logs)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
