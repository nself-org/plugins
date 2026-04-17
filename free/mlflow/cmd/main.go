package main

import (
	"log"
	"os"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-mlflow/internal"
)

func main() {
	mlflowURL := os.Getenv("MLFLOW_TRACKING_URI")
	if mlflowURL == "" {
		mlflowURL = "http://localhost:5000"
	}

	port := 3055
	if raw := os.Getenv("PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			log.Fatalf("invalid PORT value %q: %v", raw, err)
		}
		port = p
	}

	client := internal.NewMLflowClient(mlflowURL)
	h := internal.NewHandler(client)

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)
	r.Use(sdk.Logger)

	// Override the SDK default /health with our richer response.
	r.Get("/health", h.Health)

	r.Get("/v1/experiments", h.ListExperiments)
	r.Get("/v1/experiments/{id}", h.GetExperiment)
	r.Get("/v1/runs", h.ListRuns)
	r.Get("/v1/runs/{run_id}", h.GetRun)
	r.Get("/v1/models", h.ListModels)
	r.Get("/v1/models/{name}", h.GetModel)
	r.Get("/v1/metrics", h.GetMetrics)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
