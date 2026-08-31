// Package encryption verifies encryption-at-rest and TLS-in-transit status.
package encryption

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EncryptionAuditResult summarises the encryption posture of the deployment.
type EncryptionAuditResult struct {
	Timestamp    time.Time       `json:"timestamp"`
	PGCrypto     PGCryptoStatus  `json:"pgcrypto"`
	TLS          TLSStatus       `json:"tls"`
	Verdict      string          `json:"verdict"` // pass | warn | fail
	Findings     []string        `json:"findings"`
}

// PGCryptoStatus reflects pgcrypto extension availability.
type PGCryptoStatus struct {
	Enabled bool   `json:"enabled"`
	Version string `json:"version,omitempty"`
}

// TLSStatus reflects TLS posture detected from runtime env.
type TLSStatus struct {
	DatabaseTLS bool   `json:"database_tls"`
	Notes       string `json:"notes,omitempty"`
}

// AuditEncryptionHandler checks pgcrypto + TLS status and returns a verdict.
func AuditEncryptionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := EncryptionAuditResult{
			Timestamp: time.Now().UTC(),
			Findings:  []string{},
		}

		// Check pgcrypto extension.
		var extName, extVersion string
		err := pool.QueryRow(r.Context(), `
			SELECT name, default_version
			FROM pg_available_extensions
			WHERE name = 'pgcrypto' AND installed_version IS NOT NULL`,
		).Scan(&extName, &extVersion)
		if err != nil {
			result.PGCrypto.Enabled = false
			result.Findings = append(result.Findings,
				"pgcrypto extension is not installed — encryption-at-rest functions unavailable")
		} else {
			result.PGCrypto.Enabled = true
			result.PGCrypto.Version = extVersion
		}

		// Check TLS on the DB connection.
		// pgx reports whether the connection is encrypted.
		var sslInUse bool
		err = pool.QueryRow(r.Context(), `
			SELECT ssl FROM pg_stat_ssl
			WHERE pid = pg_backend_pid()`).Scan(&sslInUse)
		if err != nil {
			// pg_stat_ssl may not be available on all PostgreSQL setups.
			result.TLS.DatabaseTLS = false
			result.TLS.Notes = fmt.Sprintf("could not query pg_stat_ssl: %v", err)
			result.Findings = append(result.Findings,
				"TLS status of the database connection could not be verified")
		} else {
			result.TLS.DatabaseTLS = sslInUse
			if !sslInUse {
				result.Findings = append(result.Findings,
					"database connection is not encrypted — enable SSL on DATABASE_URL")
			}
		}

		// Verdict.
		switch len(result.Findings) {
		case 0:
			result.Verdict = "pass"
		case 1:
			result.Verdict = "warn"
		default:
			result.Verdict = "fail"
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
