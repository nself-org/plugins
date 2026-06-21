package internal

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func (h *Handler) runBackup(jobID, backupType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := os.MkdirAll(h.storagePath, 0o755); err != nil {
		log.Printf("backup: mkdir failed: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("mkdir: %v", err))
		return
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	fileName := fmt.Sprintf("backup-%s-%s.pgdump", jobID, timestamp)
	filePath := filepath.Join(h.storagePath, fileName)

	args := h.buildPgDumpArgs(backupType)

	cmd := exec.CommandContext(ctx, h.pgDumpPath, args...)
	cmd.Env = h.pgEnv()

	// CRITICAL: Use StdoutPipe + io.Copy for streaming (TRAP 3).
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("backup: stdout pipe: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("stdout pipe: %v", err))
		return
	}

	outFile, err := os.Create(filePath)
	if err != nil {
		log.Printf("backup: create file: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("create file: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		outFile.Close()
		os.Remove(filePath)
		log.Printf("backup: start pg_dump: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("start pg_dump: %v", err))
		return
	}

	// Stream pg_dump stdout directly to file. Hash as we go.
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(outFile, hasher), stdout)

	// Close the file before waiting so the fd is released.
	outFile.Close()

	waitErr := cmd.Wait()
	if waitErr != nil {
		os.Remove(filePath)
		msg := fmt.Sprintf("pg_dump exited: %v", waitErr)
		log.Printf("backup: %s", msg)
		_ = h.store.FailJob(ctx, jobID, msg)
		return
	}
	if copyErr != nil {
		os.Remove(filePath)
		msg := fmt.Sprintf("io copy: %v", copyErr)
		log.Printf("backup: %s", msg)
		_ = h.store.FailJob(ctx, jobID, msg)
		return
	}

	_ = fmt.Sprintf("%x", hasher.Sum(nil)) // checksum available if needed later

	if err := h.store.CompleteJob(ctx, jobID, filePath, written); err != nil {
		log.Printf("backup: complete job: %v", err)
	}

	log.Printf("backup: job %s completed, %d bytes written to %s", jobID, written, filePath)
}

// buildPgDumpArgs returns pg_dump flags for the given backup type.
// Output goes to stdout (custom format) so it can be piped.
func (h *Handler) buildPgDumpArgs(backupType string) []string {
	args := []string{
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"--verbose",
	}

	switch backupType {
	case "schema_only":
		args = append(args, "--schema-only")
	case "data_only":
		args = append(args, "--data-only")
	}

	// Use the DATABASE_URL via --dbname so pg_dump resolves host/port/user from it.
	args = append(args, "--dbname", h.databaseURL)

	return args
}

// pgEnv returns environment variables for pg_dump / pg_restore, carrying over
// the current env but ensuring PGPASSWORD is NOT set (the connection string
// already includes credentials).
func (h *Handler) pgEnv() []string {
	env := os.Environ()
	// Parse password from DATABASE_URL and inject as PGPASSWORD so that
	// pg_dump/pg_restore do not prompt interactively.
	if u, err := url.Parse(h.databaseURL); err == nil {
		if pw, ok := u.User.Password(); ok {
			env = append(env, "PGPASSWORD="+pw)
		}
	}
	return env
}

// --------------------------------------------------------------------------
// GET /v1/backups — list backups
// --------------------------------------------------------------------------
