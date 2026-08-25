package claw

// Purpose: shells out to `docker exec <postgres container> psql ...` to run
// claw schema migrations against the project's database.
// Constraints: moved verbatim from cli/internal/claw/db.go under CLI-R11,
// except *config.Config -> *projectenv.Config (see internal/projectenv for
// why: cli/internal/config is unreachable from this plugin module and far
// larger than the three fields this file reads off it).

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nself-org/nself-claw/internal/projectenv"
)

// postgresContainer returns the postgres container name for the project.
func postgresContainer(cfg *projectenv.Config) string {
	return cfg.ProjectName + "_postgres"
}

// postgresUser returns the postgres user from config with a safe default.
func postgresUser(cfg *projectenv.Config) string {
	if cfg.PostgresUser != "" {
		return cfg.PostgresUser
	}
	return "postgres"
}

// postgresDB returns the target database name with a safe default.
func postgresDB(cfg *projectenv.Config) string {
	if cfg.PostgresDB != "" {
		return cfg.PostgresDB
	}
	return "nself"
}

// runClawSQL pipes raw SQL into psql inside the postgres container.
// The SQL is executed against the project's database (cfg.PostgresDB or "nself").
// On error, the psql stderr is included in the returned error message.
func runClawSQL(ctx context.Context, cfg *projectenv.Config, sql string) error {
	container := postgresContainer(cfg)
	user := postgresUser(cfg)
	db := postgresDB(cfg)

	args := []string{
		"exec", "-i", container,
		"psql",
		"-U", user,
		"-d", db,
		"-v", "ON_ERROR_STOP=1",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(sql)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// queryClawSQL executes a SQL query inside the postgres container and returns
// the trimmed stdout. Useful for SELECT queries that return a single value or
// newline-separated list.
func queryClawSQL(ctx context.Context, cfg *projectenv.Config, sql string) (string, error) {
	container := postgresContainer(cfg)
	user := postgresUser(cfg)
	db := postgresDB(cfg)

	cmd := exec.CommandContext(ctx, "docker", "exec", container,
		"psql", "-U", user, "-d", db, "-tAc", sql,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
