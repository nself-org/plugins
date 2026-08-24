// Purpose: the plugin's cobra root, plus the shared openDB/strPtr helpers
// moved from cli/cmd/commands/gdpr.go (CLI-R11 extraction). ProxyCommand in
// the core CLI's internal/plugin/router.go execs this binary as
// `nself-gdpr <args...>` with the leading "gdpr" argument already stripped,
// so this root command takes the place `nself gdpr` used to occupy in the
// core binary — its subcommands (export, delete, forget, status,
// list-requests) are what `nself gdpr <sub>` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router; DATABASE_URL env
// var for openDB.
//
// Outputs: process exit code (0 success, 1 fail — including an internal
// panic caught by PersistentPreRunE's recover guard), mirroring the exit
// codes documented on `nself gdpr --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// SilenceUsage/SilenceErrors are set to match the core CLI's RootCmd
// (cmd/commands/root.go) — see the CLI-R11 pentest-kit extraction's commit
// message for why this matters on any RunE error path.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "nself-gdpr",
	Short:         "GDPR data portability and right-to-erasure (Art. 20, 17)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `GDPR compliance tools for self-hosted nSelf instances.

Subcommands:
  nself gdpr export        Export all data for a user (Art. 20 portability)
  nself gdpr delete        Delete or anonymize all data for a user (Art. 17 erasure)
  nself gdpr status        Check the status of a GDPR request
  nself gdpr list-requests List all open and completed GDPR requests

All GDPR requests are logged to np_gdpr_requests for audit purposes.
That table is append-only: rows are never deleted.`,
	// PersistentPreRunE installs a recover() guard across the entire gdpr
	// subcommand tree. Any unexpected panic is caught here, logged as an
	// internal error, and converted to a non-zero exit without crashing the
	// process in a way that suppresses the error message.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (retErr error) {
		defer func() {
			if r := recover(); r != nil {
				retErr = fmt.Errorf("gdpr: internal error (panic): %v", r)
			}
		}()
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(gdprExportCmd)
	rootCmd.AddCommand(gdprDeleteCmd)
	rootCmd.AddCommand(gdprForgetCmd)
	rootCmd.AddCommand(gdprStatusCmd)
	rootCmd.AddCommand(gdprListCmd)
}

// openDB opens a Postgres connection from DATABASE_URL.
func openDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set; run `nself env` to verify your environment")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("gdpr: open database: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// strPtr returns a pointer to a string — used for optional nullable fields.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
