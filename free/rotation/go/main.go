// Package main — nself-rotation plugin
//
// Cron-triggered secret rotation service. Polls np_secret_rotation_schedules
// for entries where next_rotation_at <= now(), generates new values, writes
// them to .env.secrets (or calls a configured rotation hook), verifies service
// health, and records each event in np_secret_rotation_events.
//
// The service depends on the cron plugin being available. It registers a job
// at startup via the cron plugin's HTTP API that fires this service's
// /rotate/tick endpoint every minute. The actual rotation work is done only
// when np_secret_rotation_schedules.next_rotation_at has passed.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3061"
	}

	enabled := os.Getenv("NSELF_SECRET_ROTATION")
	if enabled != "true" {
		log.Println("rotation: NSELF_SECRET_ROTATION not set to 'true' — plugin is loaded but passive")
	}

	svc := &rotationService{
		db:      mustOpenDB(),
		dryRun:  os.Getenv("NSELF_SECRET_ROTATION_DRY_RUN") == "true",
		enabled: enabled == "true",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", svc.handleHealth)
	mux.HandleFunc("/rotate/tick", svc.handleTick)
	mux.HandleFunc("/schedules", svc.handleListSchedules)
	mux.HandleFunc("/events", svc.handleListEvents)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("rotation: listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("rotation: server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("rotation: graceful shutdown failed: %v", err)
	}
}

// handleHealth returns HTTP 200 with a JSON body.
func (s *rotationService) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleTick is called by the cron plugin every minute. It checks which
// secrets are due for rotation and processes them.
func (s *rotationService) handleTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.enabled {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "rotation disabled")
		return
	}
	ctx := r.Context()
	rotated, skipped, errs := s.processDueSchedules(ctx)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rotated": rotated,
		"skipped": skipped,
		"errors":  errs,
	})
}

// handleListSchedules returns all rotation schedules as JSON.
func (s *rotationService) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	schedules, err := s.listSchedules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

// handleListEvents returns recent rotation events as JSON.
func (s *rotationService) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, err := s.listEvents(r.Context(), 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
