package internal

import "time"

// DSAR represents a Data Subject Access Request.
type DSAR struct {
	ID                       string     `json:"id"`
	SourceAccountID          string     `json:"source_account_id"`
	RequestType              string     `json:"request_type"`
	RequestNumber            string     `json:"request_number"`
	UserID                   *string    `json:"user_id"`
	RequesterEmail           string     `json:"requester_email"`
	RequesterName            *string    `json:"requester_name"`
	VerificationToken        *string    `json:"verification_token,omitempty"`
	VerificationSentAt       *time.Time `json:"verification_sent_at"`
	VerificationCompletedAt  *time.Time `json:"verification_completed_at"`
	VerifiedBy               *string    `json:"verified_by"`
	Description              *string    `json:"description"`
	DataCategories           []string   `json:"data_categories"`
	SpecificDataRequested    *string    `json:"specific_data_requested"`
	Status                   string     `json:"status"`
	AssignedTo               *string    `json:"assigned_to"`
	StartedAt                *time.Time `json:"started_at"`
	CompletedAt              *time.Time `json:"completed_at"`
	Deadline                 time.Time  `json:"due_date"` // field alias matches task spec
	DataPackageURL           *string    `json:"data_package_url"`
	DataPackageSizeBytes     *int64     `json:"data_package_size_bytes"`
	DataPackageGeneratedAt   *time.Time `json:"data_package_generated_at"`
	ResolutionNotes          *string    `json:"resolution_notes"`
	RejectionReason          *string    `json:"rejection_reason"`
	Regulation               string     `json:"regulation"`
	Jurisdiction             *string    `json:"jurisdiction"`
	LegalBasis               *string    `json:"legal_basis"`
	IPAddress                *string    `json:"ip_address"`
	UserAgent                *string    `json:"user_agent"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// CreateDSARRequest is the body for POST /api/v1/dsars.
type CreateDSARRequest struct {
	RequestType           string   `json:"request_type"`
	Email                 string   `json:"email"`
	Name                  *string  `json:"name"`
	UserID                *string  `json:"user_id"`
	Description           *string  `json:"description"`
	DataCategories        []string `json:"data_categories"`
	SpecificDataRequested *string  `json:"specific_data_requested"`
	Regulation            *string  `json:"regulation"`
	Jurisdiction          *string  `json:"jurisdiction"`
}

// DSARActivity represents a single activity log entry for a DSAR.
type DSARActivity struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	DSARID          string    `json:"dsar_id"`
	ActivityType    string    `json:"activity_type"`
	Description     *string   `json:"description"`
	PerformedBy     *string   `json:"performed_by"`
	PerformedByName *string   `json:"performed_by_name"`
	Metadata        any       `json:"metadata"`
	CreatedAt       time.Time `json:"created_at"`
}

// Consent represents a user consent record.
type Consent struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	UserID               string     `json:"user_id"`
	Purpose              string     `json:"purpose"`
	PurposeDescription   *string    `json:"purpose_description"`
	Status               string     `json:"status"`
	GrantedAt            *time.Time `json:"granted_at"`
	DeniedAt             *time.Time `json:"denied_at"`
	WithdrawnAt          *time.Time `json:"withdrawn_at"`
	ExpiresAt            *time.Time `json:"expires_at"`
	ConsentMethod        *string    `json:"consent_method"`
	ConsentText          *string    `json:"consent_text"`
	PrivacyPolicyVersion *string    `json:"privacy_policy_version"`
	IPAddress            *string    `json:"ip_address"`
	UserAgent            *string    `json:"user_agent"`
	Metadata             any        `json:"metadata"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// CreateConsentRequest is the body for POST /api/v1/consents.
type CreateConsentRequest struct {
	UserID               string     `json:"user_id"`
	Purpose              string     `json:"purpose"`
	Status               string     `json:"status"`
	PurposeDescription   *string    `json:"purpose_description"`
	ConsentText          *string    `json:"consent_text"`
	ConsentMethod        *string    `json:"consent_method"`
	PrivacyPolicyVersion *string    `json:"privacy_policy_version"`
	ExpiresAt            *time.Time `json:"expires_at"`
}

// RevokeConsentRequest is the body for POST /api/v1/consents/revoke.
type RevokeConsentRequest struct {
	UserID      string `json:"user_id"`
	ConsentType string `json:"consent_type"` // maps to purpose
}

// PrivacyPolicy represents a versioned privacy policy document.
type PrivacyPolicy struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	Version              string     `json:"version"`
	VersionNumber        int        `json:"version_number"`
	Title                string     `json:"title"`
	Content              string     `json:"content"`
	Summary              *string    `json:"summary"`
	ChangesSummary       *string    `json:"changes_summary"`
	IsActive             bool       `json:"is_active"`
	RequiresReacceptance bool       `json:"requires_reacceptance"`
	EffectiveFrom        time.Time  `json:"effective_from"`
	EffectiveUntil       *time.Time `json:"effective_until"`
	Language             string     `json:"language"`
	Jurisdiction         *string    `json:"jurisdiction"`
	CreatedBy            *string    `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
}

// CreatePrivacyPolicyRequest is the body for POST /api/v1/privacy-policies.
type CreatePrivacyPolicyRequest struct {
	Version              string     `json:"version"`
	VersionNumber        int        `json:"version_number"`
	Title                string     `json:"title"`
	Content              string     `json:"content"`
	Summary              *string    `json:"summary"`
	ChangesSummary       *string    `json:"changes_summary"`
	RequiresReacceptance bool       `json:"requires_reacceptance"`
	EffectiveFrom        time.Time  `json:"effective_from"`
	Language             *string    `json:"language"`
	Jurisdiction         *string    `json:"jurisdiction"`
}

// DataExport represents a user data export request.
type DataExport struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	UserID          string     `json:"user_id"`
	Format          string     `json:"format"`
	Status          string     `json:"status"`
	ExportURL       *string    `json:"export_url"`
	ExpiresAt       *time.Time `json:"expires_at"`
	ErrorMessage    *string    `json:"error_message"`
	RequestedAt     time.Time  `json:"requested_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CreateDataExportRequest is the body for POST /api/v1/data-exports.
type CreateDataExportRequest struct {
	UserID string `json:"user_id"`
	Format string `json:"format"` // "json" | "csv"
}

// StatsResponse holds aggregated compliance statistics.
type StatsResponse struct {
	DSARs struct {
		Total     int `json:"total"`
		Pending   int `json:"pending"`
		Completed int `json:"completed"`
		Overdue   int `json:"overdue"`
	} `json:"dsars"`
	Consents struct {
		Total     int `json:"total"`
		Granted   int `json:"granted"`
		Withdrawn int `json:"withdrawn"`
	} `json:"consents"`
	PrivacyPolicies struct {
		Total  int `json:"total"`
		Active int `json:"active"`
	} `json:"privacy_policies"`
	DataExports struct {
		Total    int `json:"total"`
		Pending  int `json:"pending"`
		Complete int `json:"complete"`
	} `json:"data_exports"`
}
