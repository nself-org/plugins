package main

// subscriber_notify.go — status-page subscriber notifications on incidents.
//
// Purpose: when the down-detector opens or resolves an incident, email every
//   CONFIRMED status-page subscriber of that tenant exactly once per
//   (subscriber, incident, state). The np_saas_subscriber_notices ledger row
//   is the claim: INSERT ... ON CONFLICT DO NOTHING wins the right to send;
//   an SMTP failure releases the claim (DELETE) so the next detector pass
//   retries. Unconfirmed subscribers never receive mail (filtered in SQL).
// Inputs:  tenant + incident identity from the detector (detector.go);
//   subscriber rows from np_saas_subscribers (gateway-owned schema).
// Outputs: incident-opened / incident-resolved emails via the shared email
//   pkg; claim rows in np_saas_subscriber_notices.
// Constraints: NON-FATAL — every error is logged and swallowed; the detector
//   must never block on notification problems. Tenant isolation: the SELECT
//   is scoped by tenant_id, so tenant A's incident can never mail tenant B.
//   Subscribers are notified tenant-wide (status_page_id scoping is a future
//   refinement — incidents are not linked to a specific page today).

import (
	"context"
	"log"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/email"
)

// subscriberSelectQuery pulls the tenant's confirmed subscribers. The
// confirmed filter lives in SQL so an unconfirmed address can never be
// emailed by any caller.
const subscriberSelectQuery = `
	SELECT id::text, email FROM np_saas_subscribers
	WHERE tenant_id = $1 AND confirmed = true
	ORDER BY created_at ASC`

// notifySubscribers emails every confirmed subscriber of tenantID about
// incidentID entering state ("firing"/"opened" → incident-opened template,
// "resolved" → incident-resolved). Safe to call repeatedly: the ledger claim
// makes each (subscriber, incident, state) send at most once; a failed SMTP
// send releases its claim so a later detector pass retries.
func (g *gateway) notifySubscribers(ctx context.Context, tenantID, incidentID, state, title, description, url string) {
	if g.mail == nil || g.db == nil {
		return // SMTP or DB unconfigured — notifications disabled
	}
	if incidentID == "" {
		// No incident id (incident plugin was down when the outage opened) →
		// no dedup key; skip rather than risk cross-incident collisions on "".
		log.Printf("saas-gateway: subscriber notify skipped (no incident id) tenant=%s state=%s", tenantID, state)
		return
	}

	var tmpl email.Template
	switch state {
	case "firing", "opened":
		tmpl = email.TemplateIncidentOpened
	case "resolved":
		tmpl = email.TemplateIncidentResolved
	default:
		log.Printf("saas-gateway: subscriber notify unknown state %q for incident %s", state, incidentID)
		return
	}

	rows, err := g.db.QueryContext(ctx, subscriberSelectQuery, tenantID)
	if err != nil {
		log.Printf("saas-gateway: subscriber notify select for tenant %s: %v", tenantID, err)
		return
	}
	type subscriber struct{ id, addr string }
	var subs []subscriber
	for rows.Next() {
		var s subscriber
		if err := rows.Scan(&s.id, &s.addr); err != nil {
			log.Printf("saas-gateway: subscriber notify scan: %v", err)
			continue
		}
		subs = append(subs, s)
	}
	rows.Close() //nolint:errcheck,gosec
	if err := rows.Err(); err != nil {
		log.Printf("saas-gateway: subscriber notify rows: %v", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	for _, s := range subs {
		// Claim this (subscriber, incident, state) — losing the conflict
		// means a previous pass (or a parallel instance) already sent it.
		res, err := g.db.ExecContext(ctx, `
			INSERT INTO np_saas_subscriber_notices (tenant_id, subscriber_id, incident_id, state)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (subscriber_id, incident_id, state) DO NOTHING`,
			tenantID, s.id, incidentID, state)
		if err != nil {
			log.Printf("saas-gateway: subscriber notice claim for %s: %v", s.id, err)
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // already claimed → already sent (or in flight)
		}

		if err := g.mail.Send(ctx, s.addr, tmpl, map[string]string{
			"Title":       title,
			"Description": description,
			"Time":        now,
			"URL":         url,
		}); err != nil {
			log.Printf("saas-gateway: subscriber notify send (incident %s, state %s): %v", incidentID, state, err)
			// Release the claim so the next detector pass retries this one.
			if _, derr := g.db.ExecContext(ctx, `
				DELETE FROM np_saas_subscriber_notices
				WHERE subscriber_id = $1 AND incident_id = $2 AND state = $3`,
				s.id, incidentID, state); derr != nil {
				log.Printf("saas-gateway: subscriber notice release for %s: %v", s.id, derr)
			}
		}
	}
}
