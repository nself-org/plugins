package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const donorboxBaseURL = "https://donorbox.org/api/v1"

// DonorboxClient is an HTTP client for the Donorbox REST API.
// It uses Basic Auth and respects a 1 req/sec rate limit.
type DonorboxClient struct {
	authHeader string
	httpClient *http.Client
	lastCall   time.Time
}

// NewDonorboxClient creates a client with Basic Auth credentials.
func NewDonorboxClient(email, apiKey string) *DonorboxClient {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiKey))
	return &DonorboxClient{
		authHeader: "Basic " + creds,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// rateLimit enforces a minimum 1-second gap between API calls.
func (c *DonorboxClient) rateLimit() {
	elapsed := time.Since(c.lastCall)
	if elapsed < time.Second {
		time.Sleep(time.Second - elapsed)
	}
	c.lastCall = time.Now()
}

// get performs a GET request against the Donorbox API.
func (c *DonorboxClient) get(path string, params url.Values) ([]byte, error) {
	c.rateLimit()

	u := donorboxBaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("donorbox API error (%d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// listAllPaginated fetches all pages for the given endpoint.
func listAllPaginated[T any](c *DonorboxClient, path string, extra url.Values) ([]T, error) {
	var all []T
	page := 1
	perPage := 50

	for {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("per_page", strconv.Itoa(perPage))
		for k, vs := range extra {
			for _, v := range vs {
				params.Set(k, v)
			}
		}

		data, err := c.get(path, params)
		if err != nil {
			return all, err
		}

		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return all, fmt.Errorf("decode page %d: %w", page, err)
		}

		if len(items) == 0 {
			break
		}

		all = append(all, items...)

		if len(items) < perPage {
			break
		}
		page++
	}

	return all, nil
}

// ListAllCampaigns fetches all campaigns from the Donorbox API.
func (c *DonorboxClient) ListAllCampaigns() ([]APICampaign, error) {
	log.Println("[nself-donorbox] fetching all campaigns")
	return listAllPaginated[APICampaign](c, "/campaigns", nil)
}

// ListAllDonors fetches all donors from the Donorbox API.
func (c *DonorboxClient) ListAllDonors() ([]APIDonor, error) {
	log.Println("[nself-donorbox] fetching all donors")
	return listAllPaginated[APIDonor](c, "/donors", nil)
}

// ListAllDonations fetches all donations, optionally filtered by date range.
func (c *DonorboxClient) ListAllDonations(dateFrom, dateTo string) ([]APIDonation, error) {
	log.Println("[nself-donorbox] fetching all donations")
	params := url.Values{}
	if dateFrom != "" {
		params.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		params.Set("date_to", dateTo)
	}
	return listAllPaginated[APIDonation](c, "/donations", params)
}

// ListAllPlans fetches all recurring plans from the Donorbox API.
func (c *DonorboxClient) ListAllPlans() ([]APIPlan, error) {
	log.Println("[nself-donorbox] fetching all plans")
	return listAllPaginated[APIPlan](c, "/plans", nil)
}

// ListAllEvents fetches all events from the Donorbox API.
func (c *DonorboxClient) ListAllEvents() ([]APIEvent, error) {
	log.Println("[nself-donorbox] fetching all events")
	return listAllPaginated[APIEvent](c, "/events", nil)
}

// ListAllTickets fetches all tickets from the Donorbox API.
func (c *DonorboxClient) ListAllTickets() ([]APITicket, error) {
	log.Println("[nself-donorbox] fetching all tickets")
	return listAllPaginated[APITicket](c, "/tickets", nil)
}
