package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openSubtitlesBaseURL = "https://api.opensubtitles.com/api/v1"

// OpenSubtitlesClient wraps the OpenSubtitles REST API.
type OpenSubtitlesClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewOpenSubtitlesClient creates a client with the given API key.
func NewOpenSubtitlesClient(apiKey string) *OpenSubtitlesClient {
	return &OpenSubtitlesClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ---------------------------------------------------------------------------
// OpenSubtitles response types
// ---------------------------------------------------------------------------

// OSSearchResponse is the top-level response from /subtitles.
type OSSearchResponse struct {
	Data []OSSearchResult `json:"data"`
}

// OSSearchResult is a single search result from OpenSubtitles.
type OSSearchResult struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Attributes OSSearchAttributes  `json:"attributes"`
}

// OSSearchAttributes holds the attributes of a search result.
type OSSearchAttributes struct {
	SubtitleID       string              `json:"subtitle_id"`
	Language         string              `json:"language"`
	DownloadCount    int                 `json:"download_count"`
	NewDownloadCount int                 `json:"new_download_count"`
	HearingImpaired  bool               `json:"hearing_impaired"`
	HD               bool               `json:"hd"`
	Format           string              `json:"format"`
	FPS              float64             `json:"fps"`
	Votes            int                 `json:"votes"`
	Points           int                 `json:"points"`
	Ratings          float64             `json:"ratings"`
	FromTrusted      bool               `json:"from_trusted"`
	ForeignPartsOnly bool               `json:"foreign_parts_only"`
	AITranslated     bool               `json:"ai_translated"`
	MachineTranslated bool              `json:"machine_translated"`
	UploadDate       string              `json:"upload_date"`
	Release          string              `json:"release"`
	Comments         string              `json:"comments"`
	URL              string              `json:"url"`
	FeatureDetails   *OSFeatureDetails   `json:"feature_details,omitempty"`
	Uploader         *OSUploader         `json:"uploader,omitempty"`
	Files            []OSFile            `json:"files"`
}

// OSFeatureDetails holds media details from the search result.
type OSFeatureDetails struct {
	FeatureID   int    `json:"feature_id"`
	FeatureType string `json:"feature_type"`
	Year        int    `json:"year"`
	Title       string `json:"title"`
	MovieName   string `json:"movie_name"`
	IMDBID      int    `json:"imdb_id"`
	TMDBID      int    `json:"tmdb_id"`
}

// OSUploader holds uploader info.
type OSUploader struct {
	UploaderID int    `json:"uploader_id"`
	Name       string `json:"name"`
	Rank       string `json:"rank"`
}

// OSFile is a subtitle file entry within a search result.
type OSFile struct {
	FileID   int    `json:"file_id"`
	CDNumber int    `json:"cd_number"`
	FileName string `json:"file_name"`
}

// OSDownloadResponse is the response from POST /download.
type OSDownloadResponse struct {
	Link      string `json:"link"`
	FileName  string `json:"file_name"`
	Requests  int    `json:"requests"`
	Remaining int    `json:"remaining"`
}

// ---------------------------------------------------------------------------
// Search methods
// ---------------------------------------------------------------------------

// SearchByQuery searches OpenSubtitles by text query.
func (c *OpenSubtitlesClient) SearchByQuery(query string, languages []string) ([]OSSearchResult, error) {
	if c.apiKey == "" {
		log.Println("subtitle-manager: OpenSubtitles API key not configured")
		return nil, nil
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("languages", strings.Join(languages, ","))

	reqURL := fmt.Sprintf("%s/subtitles?%s", openSubtitlesBaseURL, params.Encode())
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build search request: %w", err)
	}
	req.Header.Set("Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("subtitle-manager: OpenSubtitles search failed: %v", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("subtitle-manager: OpenSubtitles search returned %d: %s", resp.StatusCode, string(body))
		return nil, nil
	}

	var result OSSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	return result.Data, nil
}

// SearchByHash searches OpenSubtitles by file hash and byte size.
func (c *OpenSubtitlesClient) SearchByHash(moviehash string, moviebytesize int64, languages []string) ([]OSSearchResult, error) {
	if c.apiKey == "" {
		log.Println("subtitle-manager: OpenSubtitles API key not configured")
		return nil, nil
	}

	params := url.Values{}
	params.Set("moviehash", moviehash)
	params.Set("moviebytesize", strconv.FormatInt(moviebytesize, 10))
	params.Set("languages", strings.Join(languages, ","))

	reqURL := fmt.Sprintf("%s/subtitles?%s", openSubtitlesBaseURL, params.Encode())
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build hash search request: %w", err)
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("subtitle-manager: OpenSubtitles hash search failed: %v", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("subtitle-manager: OpenSubtitles hash search returned %d: %s", resp.StatusCode, string(body))
		return nil, nil
	}

	var result OSSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode hash search response: %w", err)
	}
	return result.Data, nil
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

// DownloadSubtitle downloads a subtitle file by its file_id.
// Returns the raw file bytes, or nil if the download failed.
func (c *OpenSubtitlesClient) DownloadSubtitle(fileID int) ([]byte, error) {
	if c.apiKey == "" {
		return nil, nil
	}

	// Step 1: Request download link
	bodyStr := fmt.Sprintf(`{"file_id":%d}`, fileID)
	req, err := http.NewRequest(http.MethodPost, openSubtitlesBaseURL+"/download",
		strings.NewReader(bodyStr))
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("subtitle-manager: download request failed: %v", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("subtitle-manager: download request returned %d: %s", resp.StatusCode, string(body))
		return nil, nil
	}

	var dlResp OSDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&dlResp); err != nil {
		return nil, fmt.Errorf("decode download response: %w", err)
	}

	if dlResp.Link == "" {
		return nil, nil
	}

	// Step 2: Fetch the actual subtitle file from the link
	fileReq, err := http.NewRequest(http.MethodGet, dlResp.Link, nil)
	if err != nil {
		return nil, fmt.Errorf("build file fetch request: %w", err)
	}

	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		log.Printf("subtitle-manager: file fetch failed: %v", err)
		return nil, nil
	}
	defer fileResp.Body.Close()

	if fileResp.StatusCode != http.StatusOK {
		return nil, nil
	}

	data, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read subtitle file: %w", err)
	}
	return data, nil
}
