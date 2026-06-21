package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handlers serves the e2ee key-directory REST API.
//
// Purpose:   HTTP layer over the np_e2ee_* tables. Stores/serves PUBLIC keys.
// Inputs:    JSON requests (see models.go); pgx pool.
// Outputs:   JSON responses; rows in np_e2ee_* tables.
// Constraints (CR-C critical):
//   - No endpoint accepts, returns, or persists private key material.
//   - One-time + Kyber prekeys are consumed atomically (UPDATE ... RETURNING in
//     a transaction) so a prekey can be handed out at most once (no replay window).
//   - Signed prekeys + Kyber prekeys are signature-verified before storage.
type Handlers struct {
	db  *pgxpool.Pool
	cfg *Config
}

// NewHandlers builds a Handlers with config from the environment.
func NewHandlers(db *pgxpool.Pool) *Handlers {
	return NewHandlersFromConfig(db, LoadConfig())
}

// NewHandlersFromConfig builds a Handlers with an explicit config (used in tests).
func NewHandlersFromConfig(db *pgxpool.Pool, cfg *Config) *Handlers {
	if cfg == nil {
		cfg = LoadConfig()
	}
	return &Handlers{db: db, cfg: cfg}
}

func bg() context.Context { return context.Background() }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs the full error server-side and returns a GENERIC 500 to the
// client (CR-C MED fix: never leak err.Error() — internal detail to clients).
func serverError(w http.ResponseWriter, where string, err error) {
	log.Printf("e2ee: %s: %v", where, err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// decodeB64 decodes a base64 (std) string, returning a 400-friendly error.
func decodeB64(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("empty value")
	}
	return base64.StdEncoding.DecodeString(s)
}

