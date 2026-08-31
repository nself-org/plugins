package internal

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	DB *DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *DB) *Handlers {
	return &Handlers{DB: db}
}

// sa returns the source account ID from the request header.
func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// HealthCheck returns 200 OK.
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "rom-discovery"})
}

// --- ROM Metadata ---

// ListMetadata returns all ROM metadata for a source account.
func (h *Handlers) ListMetadata(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, rom_title, rom_title_normalized, platform,
		        region, file_name, file_size_bytes, file_hash_md5, file_hash_sha256,
		        file_hash_crc32, download_url, download_source, download_url_verified_at,
		        download_url_dead, release_year, release_month, release_day, version,
		        quality_score, popularity_score, release_group, is_verified_dump,
		        is_hack, is_translation, is_homebrew, is_public_domain, game_title,
		        genre, publisher, developer, description, igdb_id, mobygames_id,
		        box_art_url, screenshot_urls, is_community_rom, community_source_url,
		        community_update_year, scraped_from, scraped_at, created_at, updated_at
		   FROM np_romdisc_metadata
		  WHERE source_account_id = $1
		  ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []ROMMetadata{}
	for rows.Next() {
		var m ROMMetadata
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.RomTitle, &m.RomTitleNormalized, &m.Platform,
			&m.Region, &m.FileName, &m.FileSizeBytes, &m.FileHashMD5, &m.FileHashSHA256,
			&m.FileHashCRC32, &m.DownloadURL, &m.DownloadSource, &m.DownloadURLVerifiedAt,
			&m.DownloadURLDead, &m.ReleaseYear, &m.ReleaseMonth, &m.ReleaseDay, &m.Version,
			&m.QualityScore, &m.PopularityScore, &m.ReleaseGroup, &m.IsVerifiedDump,
			&m.IsHack, &m.IsTranslation, &m.IsHomebrew, &m.IsPublicDomain, &m.GameTitle,
			&m.Genre, &m.Publisher, &m.Developer, &m.Description, &m.IgdbID, &m.MobygamesID,
			&m.BoxArtURL, &m.ScreenshotURLs, &m.IsCommunityRom, &m.CommunitySourceURL,
			&m.CommunityUpdateYear, &m.ScrapedFrom, &m.ScrapedAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, m)
	}
	writeJSON(w, http.StatusOK, results)
}

// GetMetadata returns a single ROM by ID.
func (h *Handlers) GetMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)

	var m ROMMetadata
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, rom_title, rom_title_normalized, platform,
		        region, file_name, file_size_bytes, file_hash_md5, file_hash_sha256,
		        file_hash_crc32, download_url, download_source, download_url_verified_at,
		        download_url_dead, release_year, release_month, release_day, version,
		        quality_score, popularity_score, release_group, is_verified_dump,
		        is_hack, is_translation, is_homebrew, is_public_domain, game_title,
		        genre, publisher, developer, description, igdb_id, mobygames_id,
		        box_art_url, screenshot_urls, is_community_rom, community_source_url,
		        community_update_year, scraped_from, scraped_at, created_at, updated_at
		   FROM np_romdisc_metadata
		  WHERE id = $1 AND source_account_id = $2`, id, sa).Scan(
		&m.ID, &m.SourceAccountID, &m.RomTitle, &m.RomTitleNormalized, &m.Platform,
		&m.Region, &m.FileName, &m.FileSizeBytes, &m.FileHashMD5, &m.FileHashSHA256,
		&m.FileHashCRC32, &m.DownloadURL, &m.DownloadSource, &m.DownloadURLVerifiedAt,
		&m.DownloadURLDead, &m.ReleaseYear, &m.ReleaseMonth, &m.ReleaseDay, &m.Version,
		&m.QualityScore, &m.PopularityScore, &m.ReleaseGroup, &m.IsVerifiedDump,
		&m.IsHack, &m.IsTranslation, &m.IsHomebrew, &m.IsPublicDomain, &m.GameTitle,
		&m.Genre, &m.Publisher, &m.Developer, &m.Description, &m.IgdbID, &m.MobygamesID,
		&m.BoxArtURL, &m.ScreenshotURLs, &m.IsCommunityRom, &m.CommunitySourceURL,
		&m.CommunityUpdateYear, &m.ScrapedFrom, &m.ScrapedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// CreateMetadata inserts a new ROM metadata record.
func (h *Handlers) CreateMetadata(w http.ResponseWriter, r *http.Request) {
	var m ROMMetadata
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	m.SourceAccountID = h.sa(r)

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_metadata
		  (source_account_id, rom_title, rom_title_normalized, platform, region, file_name,
		   file_size_bytes, file_hash_md5, file_hash_sha256, file_hash_crc32, download_url,
		   download_source, release_year, release_month, release_day, version, quality_score,
		   release_group, is_verified_dump, is_hack, is_translation, is_homebrew,
		   is_public_domain, game_title, genre, publisher, developer, description,
		   igdb_id, mobygames_id, box_art_url, screenshot_urls, is_community_rom,
		   community_source_url, community_update_year, scraped_from, scraped_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
		         $20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37)
		 RETURNING id, created_at, updated_at`,
		m.SourceAccountID, m.RomTitle, m.RomTitleNormalized, m.Platform, m.Region, m.FileName,
		m.FileSizeBytes, m.FileHashMD5, m.FileHashSHA256, m.FileHashCRC32, m.DownloadURL,
		m.DownloadSource, m.ReleaseYear, m.ReleaseMonth, m.ReleaseDay, m.Version, m.QualityScore,
		m.ReleaseGroup, m.IsVerifiedDump, m.IsHack, m.IsTranslation, m.IsHomebrew,
		m.IsPublicDomain, m.GameTitle, m.Genre, m.Publisher, m.Developer, m.Description,
		m.IgdbID, m.MobygamesID, m.BoxArtURL, m.ScreenshotURLs, m.IsCommunityRom,
		m.CommunitySourceURL, m.CommunityUpdateYear, m.ScrapedFrom, m.ScrapedAt,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// DeleteMetadata removes a ROM metadata record.
func (h *Handlers) DeleteMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)

	ct, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM np_romdisc_metadata WHERE id = $1 AND source_account_id = $2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- Download Queue ---

// ListDownloadQueue returns all download queue entries for a source account.
func (h *Handlers) ListDownloadQueue(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, rom_metadata_id, job_id, status,
		        download_started_at, download_completed_at, download_progress_percent,
		        downloaded_bytes, total_bytes, object_storage_path, checksum_verified,
		        error_message, retry_count, max_retries, created_at, updated_at
		   FROM np_romdisc_download_queue
		  WHERE source_account_id = $1
		  ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []DownloadQueue{}
	for rows.Next() {
		var dq DownloadQueue
		if err := rows.Scan(
			&dq.ID, &dq.SourceAccountID, &dq.UserID, &dq.RomMetadataID, &dq.JobID, &dq.Status,
			&dq.DownloadStartedAt, &dq.DownloadCompletedAt, &dq.DownloadProgressPct,
			&dq.DownloadedBytes, &dq.TotalBytes, &dq.ObjectStoragePath, &dq.ChecksumVerified,
			&dq.ErrorMessage, &dq.RetryCount, &dq.MaxRetries, &dq.CreatedAt, &dq.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, dq)
	}
	writeJSON(w, http.StatusOK, results)
}

// CreateDownloadQueue enqueues a download.
func (h *Handlers) CreateDownloadQueue(w http.ResponseWriter, r *http.Request) {
	var dq DownloadQueue
	if err := json.NewDecoder(r.Body).Decode(&dq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	dq.SourceAccountID = h.sa(r)

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_download_queue
		  (source_account_id, user_id, rom_metadata_id, job_id, status)
		 VALUES ($1, $2, $3, $4, COALESCE($5, 'pending'))
		 RETURNING id, created_at, updated_at`,
		dq.SourceAccountID, dq.UserID, dq.RomMetadataID, dq.JobID, dq.Status,
	).Scan(&dq.ID, &dq.CreatedAt, &dq.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, dq)
}

// UpdateDownloadQueue updates a queue entry status.
func (h *Handlers) UpdateDownloadQueue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)

	var dq DownloadQueue
	if err := json.NewDecoder(r.Body).Decode(&dq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ct, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE np_romdisc_download_queue
		    SET status = $3, download_progress_percent = $4, downloaded_bytes = $5,
		        total_bytes = $6, error_message = $7, retry_count = $8, updated_at = NOW()
		  WHERE id = $1 AND source_account_id = $2`,
		id, sa, dq.Status, dq.DownloadProgressPct, dq.DownloadedBytes,
		dq.TotalBytes, dq.ErrorMessage, dq.RetryCount,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// --- Scraper Jobs ---

// ListScraperJobs returns all scraper jobs.
func (h *Handlers) ListScraperJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, scraper_name, scraper_type, scraper_source_url, enabled,
		        last_run_at, last_run_status, last_run_duration_seconds,
		        roms_found, roms_added, roms_updated, roms_removed, errors,
		        cron_schedule, next_run_at, created_at, updated_at
		   FROM np_romdisc_scraper_jobs
		  ORDER BY scraper_name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []ScraperJob{}
	for rows.Next() {
		var sj ScraperJob
		if err := rows.Scan(
			&sj.ID, &sj.ScraperName, &sj.ScraperType, &sj.ScraperSourceURL, &sj.Enabled,
			&sj.LastRunAt, &sj.LastRunStatus, &sj.LastRunDurationSeconds,
			&sj.RomsFound, &sj.RomsAdded, &sj.RomsUpdated, &sj.RomsRemoved, &sj.Errors,
			&sj.CronSchedule, &sj.NextRunAt, &sj.CreatedAt, &sj.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, sj)
	}
	writeJSON(w, http.StatusOK, results)
}

// CreateScraperJob creates a new scraper job.
func (h *Handlers) CreateScraperJob(w http.ResponseWriter, r *http.Request) {
	var sj ScraperJob
	if err := json.NewDecoder(r.Body).Decode(&sj); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_scraper_jobs
		  (scraper_name, scraper_type, scraper_source_url, enabled, cron_schedule)
		 VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5,''), '0 3 * * *'))
		 RETURNING id, created_at, updated_at`,
		sj.ScraperName, sj.ScraperType, sj.ScraperSourceURL, sj.Enabled, sj.CronSchedule,
	).Scan(&sj.ID, &sj.CreatedAt, &sj.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, sj)
}

// UpdateScraperJob updates a scraper job.
func (h *Handlers) UpdateScraperJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var sj ScraperJob
	if err := json.NewDecoder(r.Body).Decode(&sj); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ct, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE np_romdisc_scraper_jobs
		    SET enabled = $2, cron_schedule = $3, updated_at = NOW()
		  WHERE id = $1`,
		id, sj.Enabled, sj.CronSchedule,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// --- Popularity ---

// ListPopularity returns popularity records for a source account.
func (h *Handlers) ListPopularity(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT p.id, p.rom_metadata_id, p.download_count, p.search_count, p.play_count,
		        p.archive_org_downloads, p.computed_popularity_score, p.last_score_update_at,
		        p.created_at, p.updated_at
		   FROM np_romdisc_popularity p
		   JOIN np_romdisc_metadata m ON m.id = p.rom_metadata_id
		  WHERE m.source_account_id = $1
		  ORDER BY p.computed_popularity_score DESC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []Popularity{}
	for rows.Next() {
		var p Popularity
		if err := rows.Scan(
			&p.ID, &p.RomMetadataID, &p.DownloadCount, &p.SearchCount, &p.PlayCount,
			&p.ArchiveOrgDownloads, &p.ComputedPopularityScore, &p.LastScoreUpdateAt,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, p)
	}
	writeJSON(w, http.StatusOK, results)
}

// UpsertPopularity creates or updates popularity for a ROM.
func (h *Handlers) UpsertPopularity(w http.ResponseWriter, r *http.Request) {
	var p Popularity
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_popularity
		  (rom_metadata_id, download_count, search_count, play_count,
		   archive_org_downloads, computed_popularity_score)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (rom_metadata_id) DO UPDATE
		   SET download_count = EXCLUDED.download_count,
		       search_count = EXCLUDED.search_count,
		       play_count = EXCLUDED.play_count,
		       archive_org_downloads = EXCLUDED.archive_org_downloads,
		       computed_popularity_score = EXCLUDED.computed_popularity_score,
		       last_score_update_at = NOW(),
		       updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		p.RomMetadataID, p.DownloadCount, p.SearchCount, p.PlayCount,
		p.ArchiveOrgDownloads, p.ComputedPopularityScore,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// --- Audit Log ---

// ListAuditLog returns audit log entries for a source account.
func (h *Handlers) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, action, rom_metadata_id, rom_name,
		        rom_platform, rom_source, ip_address, user_agent, details, created_at
		   FROM np_romdisc_audit_log
		  WHERE source_account_id = $1
		  ORDER BY created_at DESC
		  LIMIT 500`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []AuditLog{}
	for rows.Next() {
		var al AuditLog
		if err := rows.Scan(
			&al.ID, &al.SourceAccountID, &al.UserID, &al.Action, &al.RomMetadataID,
			&al.RomName, &al.RomPlatform, &al.RomSource, &al.IPAddress, &al.UserAgent,
			&al.Details, &al.CreatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, al)
	}
	writeJSON(w, http.StatusOK, results)
}

// CreateAuditLog writes an audit log entry.
func (h *Handlers) CreateAuditLog(w http.ResponseWriter, r *http.Request) {
	var al AuditLog
	if err := json.NewDecoder(r.Body).Decode(&al); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	al.SourceAccountID = h.sa(r)

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_audit_log
		  (source_account_id, user_id, action, rom_metadata_id, rom_name, rom_platform,
		   rom_source, ip_address, user_agent, details)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10, '{}'))
		 RETURNING id, created_at`,
		al.SourceAccountID, al.UserID, al.Action, al.RomMetadataID, al.RomName,
		al.RomPlatform, al.RomSource, al.IPAddress, al.UserAgent, al.Details,
	).Scan(&al.ID, &al.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, al)
}

// --- Legal Acceptance ---

// ListLegalAcceptance returns legal acceptance records for a source account.
func (h *Handlers) ListLegalAcceptance(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, disclaimer_version, accepted_at,
		        ip_address, user_agent
		   FROM np_romdisc_legal_acceptance
		  WHERE source_account_id = $1
		  ORDER BY accepted_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := []LegalAcceptance{}
	for rows.Next() {
		var la LegalAcceptance
		if err := rows.Scan(
			&la.ID, &la.SourceAccountID, &la.UserID, &la.DisclaimerVersion,
			&la.AcceptedAt, &la.IPAddress, &la.UserAgent,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, la)
	}
	writeJSON(w, http.StatusOK, results)
}

// CreateLegalAcceptance records a user's acceptance of legal terms.
func (h *Handlers) CreateLegalAcceptance(w http.ResponseWriter, r *http.Request) {
	var la LegalAcceptance
	if err := json.NewDecoder(r.Body).Decode(&la); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	la.SourceAccountID = h.sa(r)

	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_romdisc_legal_acceptance
		  (source_account_id, user_id, disclaimer_version, ip_address, user_agent)
		 VALUES ($1, $2, COALESCE(NULLIF($3,''), '1.0'), $4, $5)
		 ON CONFLICT (source_account_id, user_id, disclaimer_version) DO UPDATE
		   SET accepted_at = NOW()
		 RETURNING id, accepted_at`,
		la.SourceAccountID, la.UserID, la.DisclaimerVersion, la.IPAddress, la.UserAgent,
	).Scan(&la.ID, &la.AcceptedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, la)
}
