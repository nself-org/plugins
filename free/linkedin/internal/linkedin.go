package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared-utils/httpclient"
)

// linkedinClient is the package-level HTTP client used for LinkedIn OAuth
// and API calls. 30s budget per spec.
var linkedinClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

const (
	linkedInAuthURL  = "https://www.linkedin.com/oauth/v2/authorization"
	linkedInTokenURL = "https://www.linkedin.com/oauth/v2/accessToken"
	linkedInAPIBase  = "https://api.linkedin.com/v2"
)

// OAuthURL returns the LinkedIn authorization URL the user should be redirected to.
func OAuthURL(clientID, redirectURI, state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	params.Set("scope", "r_liteprofile r_emailaddress w_member_social")
	return linkedInAuthURL + "?" + params.Encode()
}

// tokenResponse is the shape of the LinkedIn token exchange response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // seconds
}

// ExchangeCode exchanges an authorization code for an access token.
func ExchangeCode(ctx context.Context, clientID, clientSecret, redirectURI, code string) (accessToken string, expiresAt time.Time, err error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linkedInTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := linkedinClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("token exchange: status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("token decode: %w", err)
	}

	expiresAt = time.Now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return tr.AccessToken, expiresAt, nil
}

// profileResponse is the minimal shape returned by /v2/me.
type profileResponse struct {
	ID        string `json:"id"`
	LocalizedFirstName string `json:"localizedFirstName"`
	LocalizedLastName  string `json:"localizedLastName"`
}

// emailResponse is the shape returned by /v2/emailAddress.
type emailResponse struct {
	Elements []struct {
		Handle struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"handle~"`
	} `json:"elements"`
}

// FetchProfile retrieves the authenticated member's basic profile and email.
func FetchProfile(ctx context.Context, accessToken string) (linkedInID, name string, email *string, err error) {
	// Fetch profile.
	profileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, linkedInAPIBase+"/me", nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("build profile request: %w", err)
	}
	profileReq.Header.Set("Authorization", "Bearer "+accessToken)

	profileResp, err := linkedinClient.Do(profileReq)
	if err != nil {
		return "", "", nil, fmt.Errorf("profile fetch: %w", err)
	}
	defer profileResp.Body.Close()

	profileBody, _ := io.ReadAll(profileResp.Body)
	if profileResp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("profile fetch: status %d: %s", profileResp.StatusCode, string(profileBody))
	}

	var pr profileResponse
	if err := json.Unmarshal(profileBody, &pr); err != nil {
		return "", "", nil, fmt.Errorf("profile decode: %w", err)
	}

	fullName := strings.TrimSpace(pr.LocalizedFirstName + " " + pr.LocalizedLastName)

	// Fetch email address (best-effort; ignore errors).
	emailAddr := fetchEmail(ctx, accessToken)

	return pr.ID, fullName, emailAddr, nil
}

// fetchEmail retrieves the authenticated member's primary email address.
// Returns nil if the request fails or no email is found.
func fetchEmail(ctx context.Context, accessToken string) *string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		linkedInAPIBase+"/emailAddress?q=members&projection=(elements*(handle~))", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := linkedinClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var er emailResponse
	if err := json.Unmarshal(body, &er); err != nil || len(er.Elements) == 0 {
		return nil
	}

	addr := er.Elements[0].Handle.EmailAddress
	if addr == "" {
		return nil
	}
	return &addr
}

// ugcPostBody is the LinkedIn UGC Share API request body.
type ugcPostBody struct {
	Author          string          `json:"author"`
	LifecycleState  string          `json:"lifecycleState"`
	SpecificContent ugcSpecific     `json:"specificContent"`
	Visibility      ugcVisibility   `json:"visibility"`
}

type ugcSpecific struct {
	ShareContent ugcShareContent `json:"com.linkedin.ugc.ShareContent"`
}

type ugcShareContent struct {
	ShareCommentary  ugcText      `json:"shareCommentary"`
	ShareMediaCategory string     `json:"shareMediaCategory"`
	Media            []ugcMedia   `json:"media,omitempty"`
}

type ugcText struct {
	Text string `json:"text"`
}

type ugcMedia struct {
	Status      string  `json:"status"`
	OriginalURL string  `json:"originalUrl"`
	Description ugcText `json:"description,omitempty"`
}

type ugcVisibility struct {
	MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

// ugcPostResponse is the shape of the LinkedIn UGC Post creation response.
type ugcPostResponse struct {
	ID string `json:"id"`
}

// PublishPost publishes a text (and optional image URL) post to LinkedIn.
// linkedInID must be the "urn:li:person:{id}" URN.
// Returns the LinkedIn post ID on success.
func PublishPost(ctx context.Context, accessToken, linkedInID, text, visibility string, imageURL *string) (string, error) {
	if visibility == "" {
		visibility = "PUBLIC"
	}

	author := "urn:li:person:" + linkedInID

	mediaCategory := "NONE"
	var media []ugcMedia
	if imageURL != nil && *imageURL != "" {
		mediaCategory = "ARTICLE"
		media = []ugcMedia{
			{
				Status:      "READY",
				OriginalURL: *imageURL,
				Description: ugcText{Text: text},
			},
		}
	}

	payload := ugcPostBody{
		Author:         author,
		LifecycleState: "PUBLISHED",
		SpecificContent: ugcSpecific{
			ShareContent: ugcShareContent{
				ShareCommentary:    ugcText{Text: text},
				ShareMediaCategory: mediaCategory,
				Media:              media,
			},
		},
		Visibility: ugcVisibility{
			MemberNetworkVisibility: visibility,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal ugc post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		linkedInAPIBase+"/ugcPosts", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("build ugcPosts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := linkedinClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ugcPosts request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("ugcPosts: status %d: %s", resp.StatusCode, string(respBody))
	}

	var pr ugcPostResponse
	if err := json.Unmarshal(respBody, &pr); err != nil {
		// LinkedIn returns the post ID in the X-RestLi-Id header on some versions.
		if id := resp.Header.Get("X-RestLi-Id"); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("decode ugcPosts response: %w", err)
	}

	return pr.ID, nil
}
