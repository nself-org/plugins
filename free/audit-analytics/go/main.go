// audit-analytics — advanced audit log analytics plugin for nSelf.
// Provides anomaly detection (z-score over np_audit_log), user behaviour heatmaps,
// privileged-action review queue, and webhook/email alerts.
// ɳSelf+ license required (NSELF_AUDIT_ANALYTICS=true).
// Real-time alerts (Enterprise) enabled via NSELF_AUDIT_ANALYTICS_REALTIME=true.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/anomaly"
	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/handlers"
	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/patterns"
)

func main() {
	if os.Getenv("NSELF_AUDIT_ANALYTICS") != "true" {
		log.Fatal("audit-analytics: NSELF_AUDIT_ANALYTICS=true is required (ɳSelf+ license)")
	}
	if os.Getenv("AUDIT_ANALYTICS_SHARED_SECRET") == "" {
		slog.Warn("audit-analytics: AUDIT_ANALYTICS_SHARED_SECRET not set — running in open-dev mode (all analytics endpoints unauthenticated)")
	}

	port := envStr("AUDIT_ANALYTICS_PORT", "3714")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("audit-analytics: DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("audit-analytics: db open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("audit-analytics: db ping: %v", err)
	}

	// Wrap db in QueryFn / ExecFn / RefreshFn adapters.
	queryFn := func(ctx context.Context, sqlStr string, args ...interface{}) ([]map[string]interface{}, error) {
		rows, err := db.QueryContext(ctx, sqlStr, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		var result []map[string]interface{}
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return nil, err
			}
			row := make(map[string]interface{}, len(cols))
			for i, c := range cols {
				row[c] = vals[i]
			}
			result = append(result, row)
		}
		return result, rows.Err()
	}

	execFn := func(ctx context.Context, sqlStr string, args ...interface{}) (int64, error) {
		res, err := db.ExecContext(ctx, sqlStr, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return n, nil
	}

	refreshFn := func(ctx context.Context, sqlStr string, args ...interface{}) error {
		_, err := db.ExecContext(ctx, sqlStr, args...)
		return err
	}

	// Build scorer config from env.
	zScore, _ := strconv.ParseFloat(envStr("NSELF_AUDIT_ANOMALY_ZSCORE", "3.0"), 64)
	if zScore <= 0 {
		zScore = 3.0
	}
	refreshSec, _ := strconv.Atoi(envStr("NSELF_AUDIT_HEATMAP_REFRESH", "3600"))
	reviewTTLSec, _ := strconv.Atoi(envStr("NSELF_AUDIT_PRIVILEGED_REVIEW_TTL", "86400"))

	scorerCfg := anomaly.ScorerConfig{
		ZScoreThreshold:     zScore,
		BulkDeleteThreshold: 50,
		AfterHoursStart:     20,
		AfterHoursEnd:       6,
		PrivilegedReviewTTL: time.Duration(reviewTTLSec) * time.Second,
	}
	alertCfg := anomaly.AlertConfigFromEnv()

	h := handlers.New(queryFn, execFn, refreshFn, scorerCfg, alertCfg)

	mux := http.NewServeMux()

	// Health endpoint.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	h.RegisterRoutes(mux)
	h.RegisterReviewRoutes(mux)

	// Background workers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshInterval := time.Duration(refreshSec) * time.Second
	heatmapRefresher := patterns.NewHeatmapRefresher(refreshFn, refreshInterval)
	go heatmapRefresher.Run(ctx)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("audit-analytics: listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("audit-analytics: server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("audit-analytics: shutdown error: %v", err)
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
