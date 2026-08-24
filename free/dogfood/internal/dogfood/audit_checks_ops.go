package dogfood

// Purpose: the infrastructure/observability half of the dogfood check list —
// container hardening, TLS, the monitoring stack, the watchdog, promotion
// cadence, `nself doctor`, admin, and queue health. Split out of audit.go
// purely to keep both files under the 300-line cap; there is no behavior
// boundary here beyond that.
//
// Inputs: a context.Context (checks shell out to docker/curl/nself) and the
// project directory for the filesystem-based checks.
//
// Outputs: AuditCheck values consumed by RunAudit in audit.go.
//
// Constraints: read-only inspection only, matching audit.go.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func checkDockerHardened(ctx context.Context, _ string) AuditCheck {
	// Check for non-root containers
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return AuditCheck{Name: "Docker hardened", Status: "warn", Message: "cannot inspect containers", Section: "security"}
	}
	rootContainers := 0
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" {
			continue
		}
		inspectCmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Config.User}}", name)
		userOut, _ := inspectCmd.Output()
		user := strings.TrimSpace(string(userOut))
		if user == "" || user == "root" || user == "0" {
			rootContainers++
		}
	}
	if rootContainers > 0 {
		return AuditCheck{Name: "Docker hardened", Status: "warn",
			Message: fmt.Sprintf("%d containers running as root", rootContainers), Section: "security"}
	}
	return AuditCheck{Name: "Docker hardened", Status: "pass", Message: "no root containers", Section: "security"}
}

func checkNginxSSL(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "-o", "/dev/null", "https://localhost")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Nginx SSL", Status: "warn", Message: "HTTPS not responding", Section: "security"}
	}
	return AuditCheck{Name: "Nginx SSL", Status: "pass", Message: "SSL active", Section: "security"}
}

func checkMonitoringAlerts(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "http://127.0.0.1:9090/api/v1/rules")
	out, err := cmd.Output()
	if err != nil {
		return AuditCheck{Name: "Monitoring alerts", Status: "warn", Message: "Prometheus not reachable", Section: "monitoring"}
	}
	if !strings.Contains(string(out), "alerting") {
		return AuditCheck{Name: "Monitoring alerts", Status: "warn", Message: "no alert rules loaded", Section: "monitoring"}
	}
	return AuditCheck{Name: "Monitoring alerts", Status: "pass", Message: "alert rules loaded", Section: "monitoring"}
}

func checkGrafanaDashboards(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "http://127.0.0.1:3000/api/search?type=dash-db")
	out, err := cmd.Output()
	if err != nil {
		return AuditCheck{Name: "Grafana dashboards", Status: "warn", Message: "Grafana not reachable", Section: "monitoring"}
	}
	if strings.TrimSpace(string(out)) == "[]" {
		return AuditCheck{Name: "Grafana dashboards", Status: "warn", Message: "no dashboards found", Section: "monitoring"}
	}
	return AuditCheck{Name: "Grafana dashboards", Status: "pass", Message: "dashboards rendering", Section: "monitoring"}
}

func checkLokiIngesting(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "http://127.0.0.1:3100/ready")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Loki ingesting", Status: "warn", Message: "Loki not ready", Section: "monitoring"}
	}
	return AuditCheck{Name: "Loki ingesting", Status: "pass", Message: "Loki ready", Section: "monitoring"}
}

func checkWatchdogRunning(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "http://127.0.0.1:9191/health")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Watchdog running", Status: "warn", Message: "watchdog not reachable", Section: "watchdog"}
	}
	return AuditCheck{Name: "Watchdog running", Status: "pass", Message: "watchdog healthy", Section: "watchdog"}
}

func checkPromotionRecent(_ context.Context, projectDir string) AuditCheck {
	dir := filepath.Join(projectDir, ".nself", "promotions")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return AuditCheck{Name: "Promotion recent", Status: "skip", Message: "no promotions recorded", Section: "promotion"}
	}
	var newest time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if time.Since(newest) > 30*24*time.Hour {
		return AuditCheck{Name: "Promotion recent", Status: "warn",
			Message: "no promotion in last 30 days", Section: "promotion"}
	}
	return AuditCheck{Name: "Promotion recent", Status: "pass", Message: "promotion used recently", Section: "promotion"}
}

func checkDoctorPasses(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "doctor")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Doctor passes", Status: "fail", Message: "nself doctor reported issues", Section: "health"}
	}
	return AuditCheck{Name: "Doctor passes", Status: "pass", Message: "doctor clean", Section: "health"}
}

func checkAdminHealth(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "http://127.0.0.1:3021/health")
	if err := cmd.Run(); err != nil {
		return AuditCheck{Name: "Admin health", Status: "skip", Message: "admin not running", Section: "admin"}
	}
	return AuditCheck{Name: "Admin health", Status: "pass", Message: "admin responsive", Section: "admin"}
}

func checkQueueHealth(ctx context.Context, _ string) AuditCheck {
	cmd := exec.CommandContext(ctx, "nself", "queue", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AuditCheck{Name: "Queue health", Status: "skip", Message: "queue plugin not installed", Section: "queue"}
	}
	if strings.Contains(string(out), "dead") {
		return AuditCheck{Name: "Queue health", Status: "warn", Message: "dead letter queue has items", Section: "queue"}
	}
	return AuditCheck{Name: "Queue health", Status: "pass", Message: "queue workers alive", Section: "queue"}
}
