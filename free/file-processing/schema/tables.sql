-- =============================================================================
-- File Processing Plugin Schema
-- Tables for file processing, thumbnails, virus scanning, and metadata
-- =============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- File Processing Jobs
-- =============================================================================

CREATE TABLE IF NOT EXISTS file_processing_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id VARCHAR(255) NOT NULL,                  -- File identifier from storage
    file_path TEXT NOT NULL,                        -- Full path to file in storage
    file_name VARCHAR(500) NOT NULL,                -- Original filename
    file_size BIGINT NOT NULL,                      -- File size in bytes
    mime_type VARCHAR(100) NOT NULL,                -- MIME type
    storage_provider VARCHAR(50) NOT NULL,          -- minio, s3, gcs, r2, azure, b2
    storage_bucket VARCHAR(255) NOT NULL,           -- Bucket/container name

    -- Processing status
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, processing, completed, failed, cancelled
    priority INTEGER DEFAULT 5,                     -- 1-10 (10 = highest priority)
    attempts INTEGER DEFAULT 0,                     -- Number of processing attempts
    max_attempts INTEGER DEFAULT 3,                 -- Max retry attempts

    -- Processing operations
    operations JSONB NOT NULL DEFAULT '[]',         -- List of operations: ["thumbnail", "optimize", "scan", "metadata"]

    -- Results
    thumbnails JSONB DEFAULT '[]',                  -- Generated thumbnail IDs
    metadata JSONB DEFAULT '{}',                    -- Extracted metadata
    scan_result JSONB,                              -- Virus scan result
    optimization_result JSONB,                      -- Optimization result

    -- Error tracking
    error_message TEXT,
    error_stack TEXT,
    last_error_at TIMESTAMP WITH TIME ZONE,

    -- Timing
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms INTEGER,                            -- Processing duration in milliseconds

    -- Queue metadata
    queue_name VARCHAR(100) DEFAULT 'default',
    scheduled_for TIMESTAMP WITH TIME ZONE,         -- For delayed processing
    webhook_url TEXT,                               -- Optional webhook for completion notification
    webhook_secret VARCHAR(255),                    -- Webhook signing secret
    callback_data JSONB,                            -- Custom data to include in callback

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_file_id ON file_processing_jobs(file_id);
CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_status ON file_processing_jobs(status);
CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_priority ON file_processing_jobs(priority DESC);
CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_created ON file_processing_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_queue ON file_processing_jobs(queue_name, status);
CREATE INDEX IF NOT EXISTS idx_file_processing_jobs_scheduled ON file_processing_jobs(scheduled_for) WHERE status = 'pending';

-- =============================================================================
-- File Thumbnails
-- =============================================================================

CREATE TABLE IF NOT EXISTS file_thumbnails (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID REFERENCES file_processing_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL,                  -- Original file identifier

    -- Thumbnail details
    thumbnail_path TEXT NOT NULL,                   -- Path to thumbnail in storage
    thumbnail_url TEXT,                             -- Public URL if available
    width INTEGER NOT NULL,                         -- Thumbnail width in pixels
    height INTEGER NOT NULL,                        -- Thumbnail height in pixels
    size_bytes BIGINT,                              -- Thumbnail file size
    format VARCHAR(20) NOT NULL,                    -- jpeg, png, webp

    -- Processing details
    source_width INTEGER,                           -- Original image width
    source_height INTEGER,                          -- Original image height
    quality INTEGER,                                -- Compression quality (0-100)
    optimization_applied BOOLEAN DEFAULT FALSE,     -- Whether optimization was applied

    -- Metadata
    generation_time_ms INTEGER,                     -- Time to generate thumbnail
    storage_provider VARCHAR(50) NOT NULL,
    storage_bucket VARCHAR(255) NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_file_thumbnails_job_id ON file_thumbnails(job_id);
CREATE INDEX IF NOT EXISTS idx_file_thumbnails_file_id ON file_thumbnails(file_id);
CREATE INDEX IF NOT EXISTS idx_file_thumbnails_dimensions ON file_thumbnails(width, height);

-- =============================================================================
-- File Virus Scans
-- =============================================================================

CREATE TABLE IF NOT EXISTS file_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID REFERENCES file_processing_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL,                  -- File identifier

    -- Scan details
    scanner VARCHAR(50) NOT NULL DEFAULT 'clamav',  -- Scanner used (clamav, etc.)
    scan_status VARCHAR(20) NOT NULL,               -- clean, infected, error, timeout

    -- Results
    is_clean BOOLEAN,                               -- Whether file is clean
    threats_found INTEGER DEFAULT 0,                -- Number of threats detected
    threat_names TEXT[],                            -- Names of detected threats
    signature_version VARCHAR(100),                 -- Virus signature database version

    -- Scan metadata
    scan_duration_ms INTEGER,                       -- Scan duration in milliseconds
    file_size_scanned BIGINT,                       -- Size of file scanned

    -- Error handling
    error_message TEXT,

    -- Timestamps
    scanned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_file_scans_job_id ON file_scans(job_id);
CREATE INDEX IF NOT EXISTS idx_file_scans_file_id ON file_scans(file_id);
CREATE INDEX IF NOT EXISTS idx_file_scans_status ON file_scans(scan_status);
CREATE INDEX IF NOT EXISTS idx_file_scans_infected ON file_scans(is_clean) WHERE is_clean = FALSE;

-- =============================================================================
-- File Metadata
-- =============================================================================

CREATE TABLE IF NOT EXISTS file_metadata (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID REFERENCES file_processing_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL UNIQUE,           -- File identifier

    -- Basic file info
    mime_type VARCHAR(100) NOT NULL,
    file_extension VARCHAR(50),
    file_size BIGINT NOT NULL,

    -- Image metadata
    width INTEGER,                                  -- Image/video width
    height INTEGER,                                 -- Image/video height
    aspect_ratio DECIMAL(10,4),                     -- Width/height ratio
    color_space VARCHAR(50),                        -- RGB, CMYK, etc.
    bit_depth INTEGER,                              -- Bits per pixel
    has_alpha BOOLEAN,                              -- Has transparency

    -- EXIF data (before stripping)
    exif_data JSONB,                                -- Original EXIF data
    camera_make VARCHAR(100),
    camera_model VARCHAR(100),
    lens_model VARCHAR(100),
    focal_length VARCHAR(50),
    aperture VARCHAR(50),
    shutter_speed VARCHAR(50),
    iso INTEGER,
    flash VARCHAR(50),
    orientation INTEGER,

    -- Location data (if present)
    gps_latitude DECIMAL(10,6),
    gps_longitude DECIMAL(10,6),
    gps_altitude DECIMAL(10,2),
    location_name TEXT,

    -- Date/time
    date_taken TIMESTAMP WITH TIME ZONE,
    date_modified TIMESTAMP WITH TIME ZONE,

    -- Video metadata
    duration_seconds DECIMAL(10,2),                 -- Video/audio duration
    video_codec VARCHAR(50),
    audio_codec VARCHAR(50),
    frame_rate DECIMAL(10,2),
    bitrate BIGINT,

    -- Audio metadata
    audio_channels INTEGER,
    sample_rate INTEGER,

    -- Document metadata
    page_count INTEGER,                             -- For PDFs and documents
    word_count INTEGER,
    author VARCHAR(255),
    title VARCHAR(500),
    subject VARCHAR(500),

    -- Hashes for duplicate detection
    md5_hash VARCHAR(32),
    sha256_hash VARCHAR(64),
    perceptual_hash VARCHAR(64),                    -- For image similarity

    -- Processing info
    exif_stripped BOOLEAN DEFAULT FALSE,            -- Whether EXIF was stripped
    metadata_extracted_at TIMESTAMP WITH TIME ZONE,
    extraction_duration_ms INTEGER,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_file_metadata_file_id ON file_metadata(file_id);
CREATE INDEX IF NOT EXISTS idx_file_metadata_mime_type ON file_metadata(mime_type);
CREATE INDEX IF NOT EXISTS idx_file_metadata_dimensions ON file_metadata(width, height);
CREATE INDEX IF NOT EXISTS idx_file_metadata_md5 ON file_metadata(md5_hash);
CREATE INDEX IF NOT EXISTS idx_file_metadata_sha256 ON file_metadata(sha256_hash);
CREATE INDEX IF NOT EXISTS idx_file_metadata_date_taken ON file_metadata(date_taken);

-- =============================================================================
-- Views for common queries
-- =============================================================================

-- Pending jobs queue (ordered by priority and creation time)
CREATE OR REPLACE VIEW file_processing_queue AS
SELECT
    id,
    file_id,
    file_name,
    mime_type,
    status,
    priority,
    operations,
    attempts,
    max_attempts,
    scheduled_for,
    created_at
FROM file_processing_jobs
WHERE status = 'pending'
  AND (scheduled_for IS NULL OR scheduled_for <= NOW())
ORDER BY priority DESC, created_at ASC;

-- Failed jobs requiring attention
CREATE OR REPLACE VIEW file_processing_failures AS
SELECT
    j.id,
    j.file_id,
    j.file_name,
    j.status,
    j.attempts,
    j.max_attempts,
    j.error_message,
    j.last_error_at,
    j.created_at
FROM file_processing_jobs j
WHERE j.status = 'failed'
   OR (j.status = 'processing' AND j.attempts >= j.max_attempts)
ORDER BY j.last_error_at DESC;

-- Infected files
CREATE OR REPLACE VIEW file_security_alerts AS
SELECT
    s.id AS scan_id,
    s.file_id,
    j.file_name,
    s.scan_status,
    s.threats_found,
    s.threat_names,
    s.scanned_at,
    j.file_path,
    j.storage_provider,
    j.storage_bucket
FROM file_scans s
JOIN file_processing_jobs j ON s.job_id = j.id
WHERE s.is_clean = FALSE
ORDER BY s.scanned_at DESC;

-- Processing statistics by status
CREATE OR REPLACE VIEW file_processing_stats AS
SELECT
    status,
    COUNT(*) AS count,
    AVG(duration_ms) AS avg_duration_ms,
    MAX(duration_ms) AS max_duration_ms,
    MIN(duration_ms) AS min_duration_ms,
    SUM(CASE WHEN attempts > 1 THEN 1 ELSE 0 END) AS retried_count
FROM file_processing_jobs
GROUP BY status;

-- Thumbnail generation statistics
CREATE OR REPLACE VIEW thumbnail_generation_stats AS
SELECT
    t.width,
    t.height,
    t.format,
    COUNT(*) AS count,
    AVG(t.generation_time_ms) AS avg_generation_time_ms,
    AVG(t.size_bytes) AS avg_size_bytes,
    SUM(t.size_bytes) AS total_size_bytes
FROM file_thumbnails t
GROUP BY t.width, t.height, t.format
ORDER BY count DESC;

-- =============================================================================
-- Functions
-- =============================================================================

-- Update job status with automatic timestamp updates
CREATE OR REPLACE FUNCTION update_job_status()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();

    -- Set started_at when processing begins
    IF NEW.status = 'processing' AND OLD.status != 'processing' THEN
        NEW.started_at = NOW();
    END IF;

    -- Set completed_at and calculate duration when completed or failed
    IF NEW.status IN ('completed', 'failed', 'cancelled') AND OLD.status NOT IN ('completed', 'failed', 'cancelled') THEN
        NEW.completed_at = NOW();
        IF NEW.started_at IS NOT NULL THEN
            NEW.duration_ms = EXTRACT(EPOCH FROM (NEW.completed_at - NEW.started_at)) * 1000;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_job_status ON file_processing_jobs;
CREATE TRIGGER trigger_update_job_status
    BEFORE UPDATE ON file_processing_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_job_status();

-- Clean up old completed jobs (retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_jobs(retention_days INTEGER DEFAULT 30)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM file_processing_jobs
    WHERE status IN ('completed', 'cancelled')
      AND completed_at < NOW() - (retention_days || ' days')::INTERVAL;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Get next job from queue
CREATE OR REPLACE FUNCTION get_next_job(p_queue_name VARCHAR DEFAULT 'default')
RETURNS UUID AS $$
DECLARE
    v_job_id UUID;
BEGIN
    SELECT id INTO v_job_id
    FROM file_processing_jobs
    WHERE status = 'pending'
      AND queue_name = p_queue_name
      AND (scheduled_for IS NULL OR scheduled_for <= NOW())
    ORDER BY priority DESC, created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED;

    IF v_job_id IS NOT NULL THEN
        UPDATE file_processing_jobs
        SET status = 'processing',
            attempts = attempts + 1,
            updated_at = NOW()
        WHERE id = v_job_id;
    END IF;

    RETURN v_job_id;
END;
$$ LANGUAGE plpgsql;
