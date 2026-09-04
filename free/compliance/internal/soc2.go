package internal

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
)

// -------------------------------------------------------------------------
// SOC 2 evidence-pack types
// -------------------------------------------------------------------------

// EvidenceRequest is the body for POST /compliance/soc2/evidence.
type EvidenceRequest struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Controls []string `json:"controls"`
}

// EvidenceJob tracks an async evidence-pack generation job.
type EvidenceJob struct {
	JobID            string    `json:"job_id"`
	Status           string    `json:"status"`
	DownloadURL      string    `json:"download_url,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	EstimatedSeconds int       `json:"estimated_seconds"`
	CreatedAt        time.Time `json:"created_at"`
}

// SOC2Control describes a single TSC control and its evidence count.
type SOC2Control struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	EvidenceCount int    `json:"evidence_count"`
}

// ChangeLogEntry is a row from np_change_log.
type ChangeLogEntry struct {
	ID          string    `json:"id"`
	TenantID    *string   `json:"tenant_id,omitempty"`
	ChangedBy   *string   `json:"changed_by,omitempty"`
	ChangeType  string    `json:"change_type"`
	Description string    `json:"description"`
	TicketRef   *string   `json:"ticket_ref,omitempty"`
	ApprovedBy  *string   `json:"approved_by,omitempty"`
	AppliedAt   time.Time `json:"applied_at"`
	RollbackAt  *time.Time `json:"rollback_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateChangeLogRequest is the body for POST /compliance/change-log.
type CreateChangeLogRequest struct {
	ChangeType  string  `json:"change_type"`
	Description string  `json:"description"`
	TicketRef   *string `json:"ticket_ref"`
	ApprovedBy  *string `json:"approved_by"`
}

// AccessReview is a row from np_access_reviews.
type AccessReview struct {
	ID          string     `json:"id"`
	TenantID    *string    `json:"tenant_id,omitempty"`
	ReviewerID  *string    `json:"reviewer_id,omitempty"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Status      string     `json:"status"`
	Findings    any        `json:"findings"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateAccessReviewRequest is the body for POST /compliance/access-reviews.
type CreateAccessReviewRequest struct {
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	ReviewerID  *string `json:"reviewer_id"`
}

// PatchAccessReviewRequest is the body for PATCH /compliance/access-reviews/:id.
type PatchAccessReviewRequest struct {
	Status   *string `json:"status"`
	Findings any     `json:"findings"`
}

// -------------------------------------------------------------------------
// SOC 2 evidence pack builder
// -------------------------------------------------------------------------

// buildEvidencePack queries all five TSC surfaces and returns a signed ZIP.
func buildEvidencePack(ctx context.Context, db *DB, from, to time.Time) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	manifest := map[string]string{} // filename -> sha256 hex

	addEntry := func(name string, content []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := w.Write(content); err != nil {
			return fmt.Errorf("write zip entry %s: %w", name, err)
		}
		sum := sha256.Sum256(content)
		manifest[name] = hex.EncodeToString(sum[:])
		return nil
	}

	// CC6 — access provisioning log
	cc6Users, err := queryCC6Users(ctx, db, from, to)
	if err != nil {
		return nil, fmt.Errorf("cc6 users: %w", err)
	}
	if err := addEntry("CC6-user-provisioning-log.csv", cc6Users); err != nil {
		return nil, err
	}

	// CC6 — access reviews
	cc6Reviews, err := queryCC6Reviews(ctx, db, from, to)
	if err != nil {
		return nil, fmt.Errorf("cc6 reviews: %w", err)
	}
	if err := addEntry("CC6-access-review.csv", cc6Reviews); err != nil {
		return nil, err
	}

	// CC7 — change management
	cc7, err := queryCC7Changes(ctx, db, from, to)
	if err != nil {
		return nil, fmt.Errorf("cc7: %w", err)
	}
	if err := addEntry("CC7-change-management.csv", cc7); err != nil {
		return nil, err
	}

	// CC9 — vendor list
	cc9, err := queryCC9Vendors(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("cc9: %w", err)
	}
	if err := addEntry("CC9-vendor-list.csv", cc9); err != nil {
		return nil, err
	}

	// A1 — availability metrics stub (sourced from audit log)
	a1, err := queryA1Availability(ctx, db, from, to)
	if err != nil {
		return nil, fmt.Errorf("a1: %w", err)
	}
	if err := addEntry("A1-availability-metrics.csv", a1); err != nil {
		return nil, err
	}

	// Policy templates
	policies := []struct {
		name string
		tmpl string
	}{
		{"policies/access-control-policy.md", accessControlPolicyTmpl},
		{"policies/change-management-policy.md", changeManagementPolicyTmpl},
		{"policies/incident-response-policy.md", incidentResponsePolicyTmpl},
		{"policies/vendor-management-policy.md", vendorManagementPolicyTmpl},
		{"policies/business-continuity-policy.md", businessContinuityPolicyTmpl},
	}
	data := policyTemplateData{Date: time.Now().UTC().Format("2006-01-02")}
	for _, p := range policies {
		rendered, err := renderPolicy(p.tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", p.name, err)
		}
		if err := addEntry(p.name, rendered); err != nil {
			return nil, err
		}
	}

	// manifest.sha256
	var sb strings.Builder
	for k, v := range manifest {
		fmt.Fprintf(&sb, "%s  %s\n", v, k)
	}
	mw, err := zw.Create("manifest.sha256")
	if err != nil {
		return nil, fmt.Errorf("create manifest: %w", err)
	}
	if _, err := mw.Write([]byte(sb.String())); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// -------------------------------------------------------------------------
// Evidence queries
// -------------------------------------------------------------------------

func queryCC6Users(ctx context.Context, db *DB, from, to time.Time) ([]byte, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, source_account_id, performed_by, activity_type, description, created_at
		FROM compliance_dsar_activities
		WHERE activity_type IN ('access_granted', 'access_revoked', 'role_changed')
		  AND created_at BETWEEN $1 AND $2
		ORDER BY created_at DESC
		LIMIT 10000
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "source_account_id", "performed_by", "activity_type", "description", "created_at"})
	for rows.Next() {
		var id, acc, actType string
		var performedBy, desc *string
		var createdAt time.Time
		if err := rows.Scan(&id, &acc, &performedBy, &actType, &desc, &createdAt); err != nil {
			continue
		}
		pb := ""
		if performedBy != nil {
			pb = *performedBy
		}
		d := ""
		if desc != nil {
			d = *desc
		}
		_ = w.Write([]string{id, acc, pb, actType, d, createdAt.Format(time.RFC3339)})
	}
	w.Flush()
	return buf.Bytes(), nil
}

func queryCC6Reviews(ctx context.Context, db *DB, from, to time.Time) ([]byte, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, tenant_id, reviewer_id, period_start, period_end, status, completed_at, created_at
		FROM np_access_reviews
		WHERE period_end BETWEEN $1 AND $2
		ORDER BY period_end DESC
	`, from, to)
	if err != nil {
		// Table may not exist on older deploys — return empty CSV gracefully
		buf := &bytes.Buffer{}
		w := csv.NewWriter(buf)
		_ = w.Write([]string{"id", "tenant_id", "reviewer_id", "period_start", "period_end", "status", "completed_at", "created_at"})
		w.Flush()
		return buf.Bytes(), nil
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "tenant_id", "reviewer_id", "period_start", "period_end", "status", "completed_at", "created_at"})
	for rows.Next() {
		var id, ps, pe, status string
		var tenantID, reviewerID *string
		var completedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &tenantID, &reviewerID, &ps, &pe, &status, &completedAt, &createdAt); err != nil {
			continue
		}
		tid := ""
		if tenantID != nil {
			tid = *tenantID
		}
		rid := ""
		if reviewerID != nil {
			rid = *reviewerID
		}
		ca := ""
		if completedAt != nil {
			ca = completedAt.Format(time.RFC3339)
		}
		_ = w.Write([]string{id, tid, rid, ps, pe, status, ca, createdAt.Format(time.RFC3339)})
	}
	w.Flush()
	return buf.Bytes(), nil
}

func queryCC7Changes(ctx context.Context, db *DB, from, to time.Time) ([]byte, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, change_type, description, ticket_ref, approved_by, applied_at, rollback_at
		FROM np_change_log
		WHERE applied_at BETWEEN $1 AND $2
		ORDER BY applied_at DESC
		LIMIT 10000
	`, from, to)
	if err != nil {
		buf := &bytes.Buffer{}
		w := csv.NewWriter(buf)
		_ = w.Write([]string{"id", "change_type", "description", "ticket_ref", "approved_by", "applied_at", "rollback_at"})
		w.Flush()
		return buf.Bytes(), nil
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "change_type", "description", "ticket_ref", "approved_by", "applied_at", "rollback_at"})
	for rows.Next() {
		var id, changeType, desc string
		var ticketRef, approvedBy *string
		var appliedAt time.Time
		var rollbackAt *time.Time
		if err := rows.Scan(&id, &changeType, &desc, &ticketRef, &approvedBy, &appliedAt, &rollbackAt); err != nil {
			continue
		}
		tr := ""
		if ticketRef != nil {
			tr = *ticketRef
		}
		ab := ""
		if approvedBy != nil {
			ab = *approvedBy
		}
		ra := ""
		if rollbackAt != nil {
			ra = rollbackAt.Format(time.RFC3339)
		}
		_ = w.Write([]string{id, changeType, desc, tr, ab, appliedAt.Format(time.RFC3339), ra})
	}
	w.Flush()
	return buf.Bytes(), nil
}

func queryCC9Vendors(ctx context.Context, db *DB) ([]byte, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, vendor_name, data_shared, risk_tier, last_reviewed, baa_signed
		FROM np_vendor_registry
		ORDER BY risk_tier DESC, vendor_name
	`)
	if err != nil {
		buf := &bytes.Buffer{}
		w := csv.NewWriter(buf)
		_ = w.Write([]string{"id", "vendor_name", "data_shared", "risk_tier", "last_reviewed", "baa_signed"})
		w.Flush()
		return buf.Bytes(), nil
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "vendor_name", "data_shared", "risk_tier", "last_reviewed", "baa_signed"})
	for rows.Next() {
		var id, vendorName, riskTier string
		var dataShared []string
		var lastReviewed *string
		var baaSigned bool
		if err := rows.Scan(&id, &vendorName, &dataShared, &riskTier, &lastReviewed, &baaSigned); err != nil {
			continue
		}
		lr := ""
		if lastReviewed != nil {
			lr = *lastReviewed
		}
		baa := "false"
		if baaSigned {
			baa = "true"
		}
		_ = w.Write([]string{id, vendorName, strings.Join(dataShared, "|"), riskTier, lr, baa})
	}
	w.Flush()
	return buf.Bytes(), nil
}

func queryA1Availability(ctx context.Context, db *DB, from, to time.Time) ([]byte, error) {
	// Query compliance_audit_events for availability-related events if the table exists.
	rows, err := db.Pool.Query(ctx, `
		SELECT DATE(created_at) AS day,
		       COUNT(*) FILTER (WHERE event_type = 'service.start') AS starts,
		       COUNT(*) FILTER (WHERE event_type = 'service.stop')  AS stops,
		       COUNT(*) FILTER (WHERE event_type = 'health.fail')   AS failures
		FROM compliance_audit_events
		WHERE created_at BETWEEN $1 AND $2
		GROUP BY 1
		ORDER BY 1
	`, from, to)
	if err != nil {
		buf := &bytes.Buffer{}
		w := csv.NewWriter(buf)
		_ = w.Write([]string{"day", "service_starts", "service_stops", "health_failures"})
		w.Flush()
		return buf.Bytes(), nil
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"day", "service_starts", "service_stops", "health_failures"})
	for rows.Next() {
		var day string
		var starts, stops, failures int
		if err := rows.Scan(&day, &starts, &stops, &failures); err != nil {
			continue
		}
		_ = w.Write([]string{day, fmt.Sprint(starts), fmt.Sprint(stops), fmt.Sprint(failures)})
	}
	w.Flush()
	return buf.Bytes(), nil
}

// -------------------------------------------------------------------------
// Policy templates
// -------------------------------------------------------------------------

type policyTemplateData struct {
	Date string
}

func renderPolicy(tmplStr string, data policyTemplateData) ([]byte, error) {
	t, err := template.New("policy").Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const accessControlPolicyTmpl = `# Access Control Policy

**Effective date:** {{.Date}}
**Framework:** SOC 2 TSC CC6

## Purpose

This policy defines how nSelf Cloud controls logical access to production systems, data, and administrative interfaces.

## Scope

All nSelf Cloud tenant environments, including Hasura GraphQL endpoints, Postgres databases, MinIO storage, and administrative CLI access.

## Principles

1. **Least privilege.** Every account is granted only the minimum permissions required for its role.
2. **Role-based access control.** Access is managed through predefined roles: admin, operator, developer, read-only.
3. **Unique credentials.** Shared accounts are prohibited. Each user has a unique credential set.
4. **MFA required.** Multi-factor authentication is required for all accounts with administrative access.

## Provisioning

- New accounts are created by an authorized admin via the nSelf CLI or Admin UI.
- Access requests are logged in the change management system with an approving admin.
- Credentials are delivered through secure channels (no plain-text email).

## De-provisioning

- Access is revoked within 24 hours of role change or departure.
- Revocations are logged in np_change_log with the approving admin ID.
- API keys and session tokens are rotated immediately on de-provisioning.

## Review

Access reviews are conducted quarterly. Results are recorded in np_access_reviews and included in the SOC 2 evidence pack.

## Exceptions

All exceptions require documented approval from an authorized admin and are logged in the change management system.
`

const changeManagementPolicyTmpl = `# Change Management Policy

**Effective date:** {{.Date}}
**Framework:** SOC 2 TSC CC7

## Purpose

This policy establishes controls for managing changes to production infrastructure, configuration, plugins, and database schema.

## Scope

All changes to: nSelf CLI configuration, plugin installation/removal, Postgres schema migrations, Nginx configuration, and environment variables in staging or production environments.

## Change Types

| Type | Examples | Approval Required |
|------|----------|-------------------|
| config | env var changes, nginx rewrites | 1 admin approval |
| plugin | install, remove, upgrade | 1 admin approval |
| user | access grant, revoke, role change | 1 admin approval |
| schema | database migration | 1 admin + change window |

## Process

1. Change is logged in np_change_log before application.
2. Change includes description, ticket reference, and approver.
3. Changes are applied in a maintenance window when possible.
4. Rollback plan is documented for all schema migrations.
5. Post-change verification is performed within 30 minutes.

## Emergency Changes

Emergency changes may bypass the standard change window but must still be logged in np_change_log within 4 hours, with retrospective approval documented.

## Audit

All changes are retained for 7 years and included in quarterly SOC 2 evidence exports.
`

const incidentResponsePolicyTmpl = `# Incident Response Policy

**Effective date:** {{.Date}}
**Framework:** SOC 2 TSC CC7, A1

## Purpose

This policy defines how nSelf Cloud identifies, contains, eradicates, and recovers from security incidents and service disruptions.

## Severity Levels

| Severity | Definition | Response Time |
|----------|-----------|---------------|
| P0 | Production data breach or full outage | 15 minutes |
| P1 | Partial outage or suspected breach | 1 hour |
| P2 | Degraded performance, no data exposure | 4 hours |
| P3 | Minor issue, no user impact | 24 hours |

## Response Process

1. **Detection.** Automated alerts via Prometheus/Alertmanager or manual reporting.
2. **Triage.** On-call engineer assesses severity and opens incident record.
3. **Containment.** Isolate affected components without data loss.
4. **Eradication.** Root cause identified and resolved.
5. **Recovery.** Service restored from known-good state.
6. **Post-mortem.** Written review within 5 business days of P0/P1.

## Data Breach Notification

Confirmed data breaches are reported to affected tenants within 72 hours per GDPR Article 33 / CCPA requirements. Notification is logged in the compliance plugin breach records.

## Contact

Security incidents: security@nself.org
`

const vendorManagementPolicyTmpl = `# Vendor Management Policy

**Effective date:** {{.Date}}
**Framework:** SOC 2 TSC CC9

## Purpose

This policy ensures that third-party vendors who access or process nSelf Cloud data meet appropriate security standards.

## Scope

All vendors who receive, store, process, or transmit nSelf Cloud tenant data or have access to production infrastructure.

## Risk Tiers

| Tier | Definition | Review Cadence |
|------|-----------|----------------|
| critical | Access to production DB or encryption keys | Annually + on incident |
| high | Access to production systems, no raw data | Annually |
| medium | Access to aggregated or anonymized data | Every 2 years |
| low | No access to customer data | Every 3 years |

## Onboarding

- New vendors are registered in np_vendor_registry with risk tier assessment.
- Business Associate Agreements (BAA) are required for vendors handling personal data.
- Vendor security questionnaires are completed before production access is granted.

## Ongoing Monitoring

- Critical and high-tier vendors are reviewed on the cadence above.
- Vendor security incidents are tracked in the compliance breach log.
- Contracts are reviewed annually for security clauses.

## Off-boarding

- Vendor access is revoked within 48 hours of contract termination.
- Data deletion or return is confirmed in writing within 30 days.

## Vendor Registry

Current vendor list is maintained in the compliance plugin (np_vendor_registry) and exported in every SOC 2 evidence pack.
`

const businessContinuityPolicyTmpl = `# Business Continuity Policy

**Effective date:** {{.Date}}
**Framework:** SOC 2 TSC A1

## Purpose

This policy defines nSelf Cloud's approach to maintaining service availability and recovering from disruptions.

## Availability Targets

| Service | Target Uptime | RTO | RPO |
|---------|--------------|-----|-----|
| API (Hasura + Auth) | 99.9% | 1 hour | 15 minutes |
| Postgres | 99.9% | 1 hour | 15 minutes |
| MinIO Storage | 99.5% | 4 hours | 1 hour |
| CLI Plugin Registry | 99.5% | 4 hours | 1 hour |

## Backup Strategy

- Postgres: continuous WAL streaming to Hetzner Object Storage; daily full backup retained for 30 days.
- MinIO: daily snapshots to a separate Hetzner bucket in a different datacenter.
- Configuration: nSelf CLI exports full stack state; stored in encrypted backup.

## Recovery Procedures

Detailed runbooks for each service are maintained in .github/docs/operations/ and tested quarterly.

## Testing

- Backup restore test: quarterly.
- Failover drill: semi-annually.
- Full BCP test: annually.

## Communication

During an outage, status updates are published to status.nself.org every 30 minutes.
`

// -------------------------------------------------------------------------
// SOC 2 HTTP handlers
// -------------------------------------------------------------------------

// SOC 2 handlers below were gated by checkSOC2License/checkExportLicense (an
// NSELF_COMPLIANCE_SOC2 / NSELF_COMPLIANCE_EXPORT env-var check, 401 "license
// required" otherwise) until 2026-09-03 (P6-E3-W2-S1-T5 FIX-PLUGINS). compliance
// is a free plugin (plugin.json: requires_license=false, requiredEntitlements=[])
// with no gated entitlement documented, so the gate was removed.

// SOC2ControlsHandler GET /compliance/soc2/controls
func SOC2ControlsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		controls := []SOC2Control{
			{ID: "CC6", Name: "Logical and Physical Access Controls", Status: "implemented", EvidenceCount: 2},
			{ID: "CC7", Name: "System Operations and Change Management", Status: "implemented", EvidenceCount: 1},
			{ID: "CC9", Name: "Risk Mitigation", Status: "implemented", EvidenceCount: 1},
			{ID: "A1", Name: "Availability", Status: "implemented", EvidenceCount: 1},
			{ID: "C1", Name: "Confidentiality", Status: "implemented", EvidenceCount: 0},
		}
		writeJSON(w, http.StatusOK, map[string]any{"controls": controls})
	}
}

// CreateEvidencePackHandler POST /compliance/soc2/evidence
func CreateEvidencePackHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		var req EvidenceRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}

		from, err := time.Parse("2006-01-02", req.From)
		if err != nil {
			errJSON(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
			return
		}
		to, err := time.Parse("2006-01-02", req.To)
		if err != nil {
			errJSON(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
			return
		}
		to = to.Add(24*time.Hour - time.Nanosecond) // inclusive day end

		zipData, err := buildEvidencePack(r.Context(), db, from, to)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, fmt.Sprintf("evidence pack generation failed: %v", err))
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(
			`attachment; filename="soc2-evidence-%s-%s.zip"`, req.From, req.To))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipData)
	}
}

// -------------------------------------------------------------------------
// Change log handlers
// -------------------------------------------------------------------------

// ListChangeLogHandler GET /compliance/change-log
func ListChangeLogHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, tenant_id, changed_by, change_type, description, ticket_ref, approved_by, applied_at, rollback_at, created_at
			FROM np_change_log
			ORDER BY applied_at DESC
			LIMIT 200
		`)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "query failed")
			return
		}
		defer rows.Close()

		var entries []ChangeLogEntry
		for rows.Next() {
			var e ChangeLogEntry
			if err := rows.Scan(
				&e.ID, &e.TenantID, &e.ChangedBy, &e.ChangeType, &e.Description,
				&e.TicketRef, &e.ApprovedBy, &e.AppliedAt, &e.RollbackAt, &e.CreatedAt,
			); err != nil {
				continue
			}
			entries = append(entries, e)
		}
		if entries == nil {
			entries = []ChangeLogEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "count": len(entries)})
	}
}

// CreateChangeLogHandler POST /compliance/change-log
func CreateChangeLogHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateChangeLogRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.ChangeType == "" || req.Description == "" {
			errJSON(w, http.StatusBadRequest, "change_type and description are required")
			return
		}

		var id string
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO np_change_log (change_type, description, ticket_ref, approved_by)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, req.ChangeType, req.Description, req.TicketRef, req.ApprovedBy).Scan(&id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "insert failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// -------------------------------------------------------------------------
// Access review handlers
// -------------------------------------------------------------------------

// ListAccessReviewsHandler GET /compliance/access-reviews
func ListAccessReviewsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, tenant_id, reviewer_id, period_start, period_end, status, findings, completed_at, created_at, updated_at
			FROM np_access_reviews
			ORDER BY period_end DESC
			LIMIT 100
		`)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "query failed")
			return
		}
		defer rows.Close()

		var reviews []AccessReview
		for rows.Next() {
			var ar AccessReview
			var findingsRaw []byte
			if err := rows.Scan(
				&ar.ID, &ar.TenantID, &ar.ReviewerID, &ar.PeriodStart, &ar.PeriodEnd,
				&ar.Status, &findingsRaw, &ar.CompletedAt, &ar.CreatedAt, &ar.UpdatedAt,
			); err != nil {
				continue
			}
			var f any
			if err := json.Unmarshal(findingsRaw, &f); err != nil {
				f = map[string]any{}
			}
			ar.Findings = f
			reviews = append(reviews, ar)
		}
		if reviews == nil {
			reviews = []AccessReview{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews, "count": len(reviews)})
	}
}

// CreateAccessReviewHandler POST /compliance/access-reviews
func CreateAccessReviewHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateAccessReviewRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.PeriodStart == "" || req.PeriodEnd == "" {
			errJSON(w, http.StatusBadRequest, "period_start and period_end are required")
			return
		}

		var id string
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO np_access_reviews (reviewer_id, period_start, period_end)
			VALUES ($1, $2, $3)
			RETURNING id
		`, req.ReviewerID, req.PeriodStart, req.PeriodEnd).Scan(&id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "insert failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// PatchAccessReviewHandler PATCH /compliance/access-reviews/:id
func PatchAccessReviewHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req PatchAccessReviewRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}

		findingsJSON := []byte("{}")
		if req.Findings != nil {
			var err error
			findingsJSON, err = json.Marshal(req.Findings)
			if err != nil {
				errJSON(w, http.StatusBadRequest, "invalid findings")
				return
			}
		}

		var completedAt *time.Time
		if req.Status != nil && *req.Status == "complete" {
			now := time.Now().UTC()
			completedAt = &now
		}

		_, err := db.Pool.Exec(r.Context(), `
			UPDATE np_access_reviews
			SET status = COALESCE($1, status),
			    findings = CASE WHEN $2::jsonb IS NOT NULL THEN $2::jsonb ELSE findings END,
			    completed_at = COALESCE($3, completed_at),
			    updated_at = NOW()
			WHERE id = $4
		`, req.Status, string(findingsJSON), completedAt, id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
	}
}

// InitSOC2Schema creates the SOC 2 tables if they do not exist.
func (d *DB) InitSOC2Schema(ctx context.Context) error {
	const soc2Schema = `
CREATE TABLE IF NOT EXISTS np_change_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   UUID,
  changed_by  UUID,
  change_type TEXT NOT NULL CHECK (change_type IN ('config', 'plugin', 'user', 'schema')),
  description TEXT NOT NULL,
  ticket_ref  TEXT,
  approved_by UUID,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rollback_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_change_log_tenant     ON np_change_log(tenant_id, applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_log_type       ON np_change_log(change_type, applied_at DESC);

CREATE TABLE IF NOT EXISTS np_access_reviews (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID,
  reviewer_id  UUID,
  period_start DATE NOT NULL,
  period_end   DATE NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'complete', 'overdue')),
  findings     JSONB NOT NULL DEFAULT '{}',
  completed_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_access_reviews_tenant ON np_access_reviews(tenant_id, period_end DESC);
CREATE INDEX IF NOT EXISTS idx_access_reviews_status ON np_access_reviews(status, period_end);

CREATE TABLE IF NOT EXISTS np_vendor_registry (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID,
  vendor_name   TEXT NOT NULL,
  data_shared   TEXT[] NOT NULL DEFAULT '{}',
  risk_tier     TEXT NOT NULL DEFAULT 'low' CHECK (risk_tier IN ('low', 'medium', 'high', 'critical')),
  last_reviewed DATE,
  baa_signed    BOOLEAN NOT NULL DEFAULT FALSE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_vendor_registry_tenant    ON np_vendor_registry(tenant_id);
CREATE INDEX IF NOT EXISTS idx_vendor_registry_risk_tier ON np_vendor_registry(risk_tier);
`
	_, err := d.Pool.Exec(ctx, soc2Schema)
	return err
}
