package crypto

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RotationJob re-encrypts all records in np_encrypted_values for a tenant
// under a new DEK per record group. The existing CMK (kmsKeyRef) is reused
// for wrapping — key rotation means fresh DEKs, not a new CMK.
//
// To actually rotate the CMK, the tenant must perform that in their KMS
// (AWS, GCP, or Vault) and then update np_kms_configs.key_ref accordingly
// before calling this job.

// RotationProgress tracks progress for a rotation job.
type RotationProgress struct {
	TenantID    uuid.UUID
	JobID       uuid.UUID
	Total       int
	Rotated     int
	Failed      int
	StartedAt   time.Time
	CompletedAt *time.Time
	Error       string
}

// Rotator re-encrypts encrypted values for a tenant.
type Rotator struct {
	db        *pgxpool.Pool
	envelope  *Envelope
	batchSize int
	rateLimit int // batches per second
}

// NewRotator creates a Rotator with the given envelope and tuning parameters.
func NewRotator(db *pgxpool.Pool, env *Envelope, batchSize, rateLimit int) *Rotator {
	if batchSize <= 0 {
		batchSize = 1000
	}
	if rateLimit <= 0 {
		rateLimit = 100
	}
	return &Rotator{db: db, envelope: env, batchSize: batchSize, rateLimit: rateLimit}
}

// Run executes the rotation job. It fetches records in batches, re-encrypts
// each one with a fresh DEK wrapped by the current CMK, then updates the row.
// It records a rotation_started event at the beginning and rotation_completed
// or rotation_failed at the end.
func (r *Rotator) Run(ctx context.Context, tenantID, triggeredBy uuid.UUID, kmsKeyRef string) (*RotationProgress, error) {
	jobID := uuid.New()
	progress := &RotationProgress{
		TenantID:  tenantID,
		JobID:     jobID,
		StartedAt: time.Now(),
	}

	// Record rotation_started event.
	if err := r.logEvent(ctx, tenantID, triggeredBy, "rotation_started", kmsKeyRef, map[string]any{
		"job_id": jobID.String(),
	}); err != nil {
		return nil, fmt.Errorf("rotate: log started event: %w", err)
	}

	// Count total records for this tenant.
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_encrypted_values WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("rotate: count records: %w", err)
	}
	progress.Total = total

	// Process in rate-limited batches.
	batchInterval := time.Second / time.Duration(r.rateLimit)
	offset := 0
	for {
		rows, err := r.db.Query(ctx,
			`SELECT id, encrypted_data, encrypted_dek, kms_key_ref, iv
			   FROM np_encrypted_values
			  WHERE tenant_id = $1
			  ORDER BY id
			  LIMIT $2 OFFSET $3`,
			tenantID, r.batchSize, offset,
		)
		if err != nil {
			progress.Error = err.Error()
			break
		}

		type encRow struct {
			id           uuid.UUID
			ciphertext   []byte
			encryptedDEK []byte
			kmsKeyRef    string
			iv           []byte
		}
		var batch []encRow
		for rows.Next() {
			var rec encRow
			if err := rows.Scan(&rec.id, &rec.ciphertext, &rec.encryptedDEK, &rec.kmsKeyRef, &rec.iv); err != nil {
				rows.Close()
				progress.Failed++
				continue
			}
			batch = append(batch, rec)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		for _, rec := range batch {
			// Decrypt with old DEK.
			plain, err := r.envelope.Decrypt(ctx, EncryptedBundle{
				Ciphertext:   rec.ciphertext,
				EncryptedDEK: rec.encryptedDEK,
				KMSKeyRef:    rec.kmsKeyRef,
				IV:           rec.iv,
				Algorithm:    "AES-256-GCM",
			})
			if err != nil {
				progress.Failed++
				continue
			}

			// Re-encrypt with fresh DEK wrapped by current CMK.
			bundle, err := r.envelope.Encrypt(ctx, plain, kmsKeyRef)
			zeroBytes(plain)
			if err != nil {
				progress.Failed++
				continue
			}

			// Update the record.
			_, err = r.db.Exec(ctx,
				`UPDATE np_encrypted_values
				    SET encrypted_data = $1,
				        encrypted_dek  = $2,
				        kms_key_ref    = $3,
				        iv             = $4
				  WHERE id = $5`,
				bundle.Ciphertext, bundle.EncryptedDEK, bundle.KMSKeyRef, bundle.IV, rec.id,
			)
			if err != nil {
				progress.Failed++
				continue
			}
			progress.Rotated++
		}

		offset += len(batch)
		if len(batch) < r.batchSize {
			break // last batch
		}

		// Rate limiting.
		select {
		case <-ctx.Done():
			progress.Error = ctx.Err().Error()
			goto done
		case <-time.After(batchInterval):
		}
	}

done:
	now := time.Now()
	progress.CompletedAt = &now

	eventType := "rotation_completed"
	if progress.Error != "" || progress.Failed > 0 {
		eventType = "rotation_failed"
	}
	_ = r.logEvent(ctx, tenantID, triggeredBy, eventType, kmsKeyRef, map[string]any{
		"job_id":  jobID.String(),
		"total":   progress.Total,
		"rotated": progress.Rotated,
		"failed":  progress.Failed,
		"error":   progress.Error,
	})

	if progress.Failed > 0 && progress.Rotated == 0 {
		return progress, fmt.Errorf("rotate: all %d records failed", progress.Failed)
	}
	return progress, nil
}

func (r *Rotator) logEvent(ctx context.Context, tenantID, triggeredBy uuid.UUID, eventType, keyRef string, details map[string]any) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO np_encryption_key_events
		   (tenant_id, event_type, key_ref, triggered_by, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, eventType, keyRef, triggeredBy, details,
	)
	return err
}
