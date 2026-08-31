// nself-eval-gate plugin — eval harness and autonomy-tier gate for nSelf.
// Purpose: HTTP plugin providing eval suite execution, scorer management, and tier gate checks.
// Inputs: NSELF_EVAL_GATE_* env vars; pgx/v5 pool from NSELF_EVAL_GATE_DB_URL.
// Outputs: HTTP API on NSELF_EVAL_GATE_PORT (default 3770).
// Constraints: Uses nSelf plugin JWT auth; no public exposure.
//
// Port: 3770 (registered in F10-PORT-REGISTRY.md)
// SPORT: REGISTRY-PLUGINS.md nself-eval-gate row
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-eval-gate/internal/api"
	"github.com/nself-org/nself-eval-gate/internal/cache"
	"github.com/nself-org/nself-eval-gate/internal/db"
)

// Config holds all runtime configuration for the plugin.
// Purpose: Typed configuration parsed from env vars at startup.
// Inputs: NSELF_EVAL_GATE_* environment variables.
// Outputs: passed to server, store, and scorer constructors.
// Constraints: NSELF_EVAL_GATE_DB_URL is required; all other fields have defaults.
type Config struct {
	Port            string
	DBUrl           string
	RedisURL        string
	JudgeModel      string
	JudgeTimeoutS   int
	EmbedTimeoutS   int
	MaxConcurrency  int
	CacheEmbedTTLH  int
	CacheJudgeTTLH  int
	RecallK         int
	SchemaVersion   string
}

func loadConfig() Config {
	cfg := Config{
		Port:           getEnv("NSELF_EVAL_GATE_PORT", "3770"),
		DBUrl:          mustEnv("NSELF_EVAL_GATE_DB_URL"),
		RedisURL:       getEnv("NSELF_EVAL_GATE_REDIS_URL", ""),
		JudgeModel:     getEnv("NSELF_EVAL_GATE_JUDGE_MODEL", "claude-3-5-haiku-20241022"),
		JudgeTimeoutS:  getEnvInt("NSELF_EVAL_GATE_JUDGE_TIMEOUT_S", 30),
		EmbedTimeoutS:  getEnvInt("NSELF_EVAL_GATE_EMBED_TIMEOUT_S", 10),
		MaxConcurrency: getEnvInt("NSELF_EVAL_GATE_MAX_CONCURRENCY", 4),
		CacheEmbedTTLH: getEnvInt("NSELF_EVAL_GATE_CACHE_EMBED_TTL_H", 24),
		CacheJudgeTTLH: getEnvInt("NSELF_EVAL_GATE_CACHE_JUDGE_TTL_H", 1),
		RecallK:        getEnvInt("NSELF_EVAL_RECALL_K", 3),
		SchemaVersion:  getEnv("NSELF_EVAL_SCHEMA_VERSION", "1"),
	}
	return cfg
}

func main() {
	cfg := loadConfig()

	// Initialize Postgres pool.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := pgxpool.New(ctx, cfg.DBUrl)
	cancel()
	if err != nil {
		log.Fatalf("nself-eval-gate: failed to connect to DB: %v", err)
	}
	defer pool.Close()

	store := db.NewPostgresStore(pool)
	evalCache := cache.NewEvalCache(cfg.RedisURL)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check (no auth required).
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(r.Context()); err != nil {
			http.Error(w, `{"status":"unhealthy","db":"down"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","plugin":"nself-eval-gate"}`)
	})

	// Eval API routes (JWT auth middleware placeholder — wired per nSelf plugin convention).
	r.Route("/eval", func(r chi.Router) {
		r.Post("/run", api.HandleEvalRun(store, cfg.MaxConcurrency, cfg.JudgeModel,
			time.Duration(cfg.JudgeTimeoutS)*time.Second,
			time.Duration(cfg.EmbedTimeoutS)*time.Second,
			evalCache))
		r.Get("/runs/{id}", api.HandleGetRun(store))
		r.Get("/suites", api.HandleListSuites(store))
		r.Post("/validate", api.HandleValidate())
		r.Get("/thresholds", api.HandleListThresholds(store))
		r.Get("/gate/{tier}", api.HandleGateCheck(store))
	})

	addr := ":" + cfg.Port
	log.Printf("nself-eval-gate starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("nself-eval-gate: server error: %v", err)
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("nself-eval-gate: required env var %s is not set", key)
	}
	return v
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
