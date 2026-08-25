// Package dogfood implements the nSelf dogfood audit system for production
// health verification.
//
// Purpose: this file defines the audit report shape and the checks covering
// backups, disaster recovery, tenancy, licensing, billing, secrets, and the
// database layer. The remaining infrastructure/observability checks live in
// audit_checks_ops.go so neither file crosses the 300-line cap.
//
// Inputs: a project directory path and a context.Context for the checks that
// shell out to nself/docker/curl.
//
// Outputs: AuditCheck values with a pass/fail/warn/skip verdict, aggregated
// by RunAudit into an AuditReport.
//
// Constraints: every check must be safe to run repeatedly against a live
// production stack — read-only inspection only, never a mutating command.
package dogfood

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AuditCheck represents a single dogfood audit check.
type AuditCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, fail, warn, skip
	Message string `json:"message"`
	Section string `json:"section"`
}

// AuditReport holds the complete dogfood audit results.
type AuditReport struct {
	ID        string       `json:"id"`
	RanAt     time.Time    `json:"ran_at"`
	Checks    []AuditCheck `json:"checks"`
	PassCount int          `json:"pass_count"`
	FailCount int          `json:"fail_count"`
	WarnCount int          `json:"warn_count"`
	SkipCount int          `json:"skip_count"`
}

// RunAudit executes all 21 dogfood checks against the current environment.
func RunAudit(ctx context.Context, projectDir string) *AuditReport {
	report := &AuditReport{
		ID:    fmt.Sprintf("dogfood-%d", time.Now().UnixNano()),
		RanAt: time.Now(),
	}

	checks := []func(context.Context, string) AuditCheck{
		checkBackupRecency,
		checkBackupVerify,
		checkDRRunbook,
		checkMultiTenancy,
		checkLicenseValid,
		checkStripeWebhook,
		checkSecretsAudit,
		checkMigrationHealth,
		checkStagingHealth,
		checkHasuraMetadata,
		checkDBSeeds,
		checkDockerHardened,
		checkNginxSSL,
		checkMonitoringAlerts,
		checkGrafanaDashboards,
		checkLokiIngesting,
		checkWatchdogRunning,
		checkPromotionRecent,
		checkDoctorPasses,
		checkAdminHealth,
		checkQueueHealth,
	}

	for _, check := range checks {
		result := check(ctx, projectDir)
		report.Checks = append(report.Checks, result)
		switch result.Status {
		case "pass":
			report.PassCount++
		case "fail":
			report.FailCount++
		case "warn":
			report.WarnCount++
		case "skip":
			report.SkipCount++
		}
	}

	return report
}

func checkBackupRecency(_ context.Context, projectDir string) AuditCheck {
	backupDir := filepath.Join(projectDir, ".nself", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) == 0 {
		return AuditCheck{Name: "Backup recency", Status: "fail", Message: "no backups found", Section: "backups"}
	}
	var newest time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	age := time.Since(newest)
	if age > 26*time.Hour {
		return AuditCheck{Name: "Backup recency", Status: "fail",
			Message: fmt.Sprintf("last backup %s ago (>26h)", age.Round(time.Minute)), Section: "backups"}
	}
	return AuditCheck{Name: "Backup recency", Status: "pass",
		Message: fmt.Sprintf("last backup %s ago", age.Round(time.Minute)), Section: "backups"}
}

func checkBackupVerify(_ context.Context, projectDir string) AuditCheck {
	verifyDir := filepath.Join(projectDir, ".nself", "backups", "verify")
	entries, err := os.ReadDir(verifyDir)
	if err != nil || len(entries) == 0 {
		return AuditCheck{Name: "Backup verify", Status: "warn", Message: "no verify records found", Section: "backups"}
	}
	var newest time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	age := time.Since(newest)
	if age > 8*24*time.Hour {
		return AuditCheck{Name: "Backup verify", Status: "fail",
			Message: fmt.Sprintf("last verify %s ago (>8d)", age.Round(time.Hour)), Section: "backups"}
	}
	return AuditCheck{Name: "Backup verify", Status: "pass",
		Message: fmt.Sprintf("last verify %s ago", age.Round(time.Hour)), Section: "backups"}
}

func checkDRRunbook(_ context.Context, projectDir string) AuditCheck {
	paths := []string{
		filepath.Join(projectDir, ".nself", "dr-runbook.md"),
		filepath.Join(projectDir, "dr-runbook.md"),
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		age := time.Since(info.ModTime())
		if age > 90*24*time.Hour {
			return AuditCheck{Name: "DR runbook", Status: "warn",
				Message: fmt.Sprintf("runbook not reviewed in %d days", int(age.Hours()/24)), Section: "dr"}
		}
		return AuditCheck{Name: "DR runbook", Status: "pass", Message: "exists and reviewed recently", Section: "dr"}
	}
	return AuditCheck{Name: "DR runbook", Status: "warn", Message: "no DR runbook found", Section: "dr"}
}

func checkMultiTenancy(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "tenant", "list")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Multi-tenancy", Status: "skip", Message: "tenant command not available", Section: "tenancy"}
	}
	return AuditCheck{Name: "Multi-tenancy", Status: "pass", Message: "tenant system active", Section: "tenancy"}
}

func checkLicenseValid(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "license", "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AuditCheck{Name: "License validation", Status: "warn", Message: "license check failed", Section: "license"}
	}
	if strings.Contains(string(out), "expired") {
		return AuditCheck{Name: "License validation", Status: "fail", Message: "license expired", Section: "license"}
	}
	return AuditCheck{Name: "License validation", Status: "pass", Message: "license valid", Section: "license"}
}

func checkStripeWebhook(_ context.Context, _ string) AuditCheck {
	// This would check Stripe webhook delivery status via API
	return AuditCheck{Name: "Stripe webhook", Status: "skip", Message: "requires Stripe API access", Section: "billing"}
}

func checkSecretsAudit(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "secrets", "audit")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Secrets audit", Status: "warn", Message: "secrets audit returned issues", Section: "security"}
	}
	return AuditCheck{Name: "Secrets audit", Status: "pass", Message: "secrets audit clean", Section: "security"}
}

func checkMigrationHealth(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "db", "migrate", "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AuditCheck{Name: "Migration health", Status: "warn",
			Message: "cannot check migration status", Section: "database"}
	}
	if strings.Contains(string(out), "pending") {
		return AuditCheck{Name: "Migration health", Status: "warn",
			Message: "pending migrations exist", Section: "database"}
	}
	return AuditCheck{Name: "Migration health", Status: "pass", Message: "all migrations applied", Section: "database"}
}

func checkStagingHealth(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "doctor", "--env", "staging")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Staging health", Status: "warn", Message: "staging environment unhealthy", Section: "environments"}
	}
	return AuditCheck{Name: "Staging health", Status: "pass", Message: "staging healthy", Section: "environments"}
}

func checkHasuraMetadata(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "db", "metadata", "diff")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AuditCheck{Name: "Hasura metadata", Status: "warn", Message: "cannot check metadata", Section: "database"}
	}
	if strings.TrimSpace(string(out)) != "" && !strings.Contains(string(out), "no diff") {
		return AuditCheck{Name: "Hasura metadata", Status: "warn", Message: "metadata drift detected", Section: "database"}
	}
	return AuditCheck{Name: "Hasura metadata", Status: "pass", Message: "no metadata drift", Section: "database"}
}

func checkDBSeeds(_ context.Context, projectDir string) AuditCheck {
	seedDir := filepath.Join(projectDir, "seeds")
	if _, err := os.Stat(seedDir); err != nil {
		return AuditCheck{Name: "DB seeds", Status: "skip", Message: "no seeds directory", Section: "database"}
	}
	return AuditCheck{Name: "DB seeds", Status: "pass", Message: "seeds directory present", Section: "database"}
}
