// Purpose: `nself-gdpr status` and `nself-gdpr list-requests` — moved
// verbatim from cli/cmd/commands/gdpr_query.go (CLI-R11 extraction).
// Read-only lookups against the np_gdpr_requests audit table.
//
// Inputs: cobra command flags (--request, --status).
//
// Outputs: a single request's detail, or a table of matching requests.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nself-org/nself-gdpr/internal/gdpr"
	"github.com/nself-org/nself-gdpr/internal/tui"

	"github.com/spf13/cobra"
)

var gdprStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of a GDPR request",
	RunE:  runGDPRStatus,
}

func init() {
	gdprStatusCmd.Flags().String("request", "", "Request ID to look up (required)")
	if err := gdprStatusCmd.MarkFlagRequired("request"); err != nil {
		// Programming error: MarkFlagRequired only returns an error when the named
		// flag does not exist on the command. Since "request" is registered on the
		// line above, this can only fire if this code is misedited. It is a
		// bug-in-our-code guard, not a user-input boundary.
		log.Fatalf("gdpr status: mark --request required: %v — this is a code bug, not a config error", err)
	}
}

func runGDPRStatus(cmd *cobra.Command, _ []string) error {
	requestID, _ := cmd.Flags().GetString("request")

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	req, err := gdpr.GetRequest(cmd.Context(), db, requestID)
	if err != nil {
		return err
	}

	tui.Info(fmt.Sprintf("Request: %s", req.ID))
	tui.Info(fmt.Sprintf("Type:    %s / %s", req.RequestType, req.SubjectType))
	tui.Info(fmt.Sprintf("Subject: %s", req.SubjectID))
	tui.Info(fmt.Sprintf("Status:  %s", req.Status))
	tui.Info(fmt.Sprintf("Created: %s", req.RequestedAt.Format(time.RFC3339)))
	tui.Info(fmt.Sprintf("Deadline:%s", req.Deadline.Format("2006-01-02")))
	if req.ArtifactURL != nil {
		tui.Info(fmt.Sprintf("Archive: %s", *req.ArtifactURL))
	}
	if req.Notes != nil && *req.Notes != "" {
		tui.Info(fmt.Sprintf("Notes:   %s", *req.Notes))
	}
	return nil
}

var gdprListCmd = &cobra.Command{
	Use:   "list-requests",
	Short: "List all GDPR requests",
	RunE:  runGDPRList,
}

func init() {
	gdprListCmd.Flags().String("status", "", "Filter by status: pending, processing, complete, failed")
}

func runGDPRList(cmd *cobra.Command, _ []string) error {
	statusFilter, _ := cmd.Flags().GetString("status")

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	requests, err := gdpr.ListRequests(cmd.Context(), db, statusFilter)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		tui.Info("No GDPR requests found.")
		return nil
	}

	tui.Info(fmt.Sprintf("%-36s  %-8s  %-8s  %-12s  %-10s  %s",
		"ID", "TYPE", "SUBJECT", "STATUS", "DEADLINE", "SUBJECT_ID"))
	for _, r := range requests {
		tui.Info(fmt.Sprintf("%-36s  %-8s  %-8s  %-12s  %-10s  %s",
			r.ID, r.RequestType, r.SubjectType, r.Status,
			r.Deadline.Format("2006-01-02"), r.SubjectID))
	}
	return nil
}
