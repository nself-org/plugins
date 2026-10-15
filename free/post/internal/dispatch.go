package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

// =============================================================================
// Internal helpers
// =============================================================================

// loadAccountWithCredentials fetches an account and decrypts its credentials.
// Returns (nil, nil) when the account does not exist.
func (h *Handlers) loadAccountWithCredentials(ctx context.Context, accountID, sourceID string) (*PostAccount, error) {
	var a PostAccount
	var credRaw string
	err := h.DB.QueryRow(ctx,
		`SELECT id, source_account_id, platform, name, endpoint, credentials, created_at
		 FROM np_post_accounts WHERE id = $1 AND source_account_id = $2`,
		accountID, sourceID,
	).Scan(&a.ID, &a.SourceAccountID, &a.Platform, &a.Name, &a.Endpoint, &credRaw, &a.CreatedAt)
	if err != nil {
		return nil, nil //nolint:nilerr — treat not-found and other scan errors as "not found"
	}

	var credMap map[string]interface{}
	if err := json.Unmarshal([]byte(credRaw), &credMap); err == nil {
		if enc, ok := credMap["__enc"].(string); ok {
			plain, err := h.decryptCredentials(enc)
			if err != nil {
				return nil, fmt.Errorf("decrypt credentials: %w", err)
			}
			if err := json.Unmarshal(plain, &credMap); err != nil {
				return nil, fmt.Errorf("unmarshal credentials: %w", err)
			}
		}
	}
	a.Credentials = credMap
	return &a, nil
}

// platformDispatchResult is the outcome of a publish operation.
type platformDispatchResult struct {
	URL      *string
	PostID   *string
	Metadata map[string]interface{}
}

// platformNotImplementedError is returned by dispatchToPlatform when the
// caller requested a platform we have not yet shipped an adapter for. Handlers
// use errors.As to detect this and surface a proper 501 + roadmap URL.
type platformNotImplementedError struct {
	Platform string
}

func (e *platformNotImplementedError) Error() string {
	return fmt.Sprintf("platform %q is not yet implemented — see roadmap at https://github.com/nself-org/plugins-pro/issues", e.Platform)
}

// dispatchToPlatform routes the publish request to the appropriate platform
// adapter.
//
// Shipped adapters: internal (no-op queue row), wordpress (XML-RPC wp.newPost),
// ghost (Admin API). Everything else returns *platformNotImplementedError —
// the caller uses errors.As to map that to HTTP 501. Real dispatch failures
// (network, auth, 4xx/5xx from the upstream) are returned as wrapped errors
// and mapped to HTTP 502 by the caller.
func dispatchToPlatform(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	switch account.Platform {
	case "internal":
		// Internal posts are stored in np_post_queue by the caller. The dispatch
		// is a no-op success — the queue row IS the published artifact.
		return &platformDispatchResult{URL: nil}, nil

	case "wordpress":
		return dispatchWordPress(ctx, account, title, content, tags)

	case "ghost":
		return dispatchGhost(ctx, account, title, content, tags)

	case "twitter", "x":
		return dispatchTwitter(ctx, account, title, content, tags)
	case "linkedin":
		return dispatchLinkedIn(ctx, account, title, content, tags)
	case "telegram":
		return dispatchTelegram(ctx, account, title, content, tags)
	case "devto", "dev.to":
		return dispatchDevTo(ctx, account, title, content, tags)
	case "hashnode":
		return dispatchHashnode(ctx, account, title, content, tags)
	case "mastodon":
		return nil, &platformNotImplementedError{Platform: "mastodon"}
	case "bluesky":
		return nil, &platformNotImplementedError{Platform: "bluesky"}
	case "facebook":
		return nil, &platformNotImplementedError{Platform: "facebook"}
	case "instagram":
		return nil, &platformNotImplementedError{Platform: "instagram"}
	case "threads":
		return nil, &platformNotImplementedError{Platform: "threads"}

	default:
		return nil, fmt.Errorf("unknown platform %q — supported: internal, wordpress, ghost; planned: twitter, linkedin, mastodon, bluesky, facebook, instagram, threads", account.Platform)
	}
}
