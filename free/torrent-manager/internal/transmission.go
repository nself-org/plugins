package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// TransmissionClient is an HTTP client for the Transmission RPC API.
type TransmissionClient struct {
	baseURL   string
	username  string
	password  string
	sessionID string
	mu        sync.Mutex
	client    *http.Client
}

// NewTransmissionClient creates a new Transmission RPC client.
func NewTransmissionClient(host string, port int, username, password string) *TransmissionClient {
	return &TransmissionClient{
		baseURL:  fmt.Sprintf("http://%s:%d/transmission/rpc", host, port),
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --- RPC request/response types ---

type rpcRequest struct {
	Method    string      `json:"method"`
	Arguments interface{} `json:"arguments,omitempty"`
	Tag       int         `json:"tag,omitempty"`
}

type rpcResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments"`
	Tag       int             `json:"tag"`
}

type addTorrentArgs struct {
	Filename    string `json:"filename"`
	DownloadDir string `json:"download-dir,omitempty"`
	Paused      bool   `json:"paused"`
}

type torrentAddedResult struct {
	TorrentAdded *transmissionTorrent `json:"torrent-added"`
}

type torrentGetResult struct {
	Torrents []transmissionTorrent `json:"torrents"`
}

type sessionStatsResult struct {
	ActiveTorrentCount int `json:"activeTorrentCount"`
	PausedTorrentCount int `json:"pausedTorrentCount"`
	TorrentCount       int `json:"torrentCount"`
}

// transmissionTorrent represents a torrent as returned by Transmission RPC.
type transmissionTorrent struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	HashString        string  `json:"hashString"`
	MagnetLink        string  `json:"magnetLink"`
	Status            int     `json:"status"`
	TotalSize         int64   `json:"totalSize"`
	DownloadedEver    int64   `json:"downloadedEver"`
	UploadedEver      int64   `json:"uploadedEver"`
	PercentDone       float64 `json:"percentDone"`
	UploadRatio       float64 `json:"uploadRatio"`
	RateDownload      int64   `json:"rateDownload"`
	RateUpload        int64   `json:"rateUpload"`
	PeersSendingToUs  int     `json:"peersSendingToUs"`
	PeersGettingFromUs int    `json:"peersGettingFromUs"`
	PeersConnected    int     `json:"peersConnected"`
	DownloadDir       string  `json:"downloadDir"`
	AddedDate         int64   `json:"addedDate"`
	ActivityDate      int64   `json:"activityDate"`
	DoneDate          int64   `json:"doneDate"`
	Error             int     `json:"error"`
	ErrorString       string  `json:"errorString"`
}

// torrentFields is the list of fields to request from Transmission.
var torrentFields = []string{
	"id", "name", "hashString", "magnetLink", "status",
	"totalSize", "downloadedEver", "uploadedEver", "percentDone", "uploadRatio",
	"rateDownload", "rateUpload", "peersSendingToUs", "peersGettingFromUs", "peersConnected",
	"downloadDir", "addedDate", "activityDate", "doneDate", "error", "errorString",
}

// --- Public methods ---

// Connect tests the connection to Transmission by requesting session info.
func (t *TransmissionClient) Connect() error {
	_, err := t.doRPC("session-get", nil)
	return err
}

// IsConnected tests whether the Transmission daemon is reachable.
func (t *TransmissionClient) IsConnected() bool {
	err := t.Connect()
	return err == nil
}

// AddTorrent adds a torrent by magnet URI and returns the added torrent info.
func (t *TransmissionClient) AddTorrent(magnetURI, downloadDir string) (*transmissionTorrent, error) {
	args := addTorrentArgs{
		Filename:    magnetURI,
		DownloadDir: downloadDir,
		Paused:      false,
	}

	resp, err := t.doRPC("torrent-add", args)
	if err != nil {
		return nil, fmt.Errorf("add torrent: %w", err)
	}

	var result torrentAddedResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse add response: %w", err)
	}

	if result.TorrentAdded == nil {
		return nil, fmt.Errorf("torrent-added field missing from response")
	}

	return result.TorrentAdded, nil
}

// GetTorrent returns a single torrent by its Transmission ID.
func (t *TransmissionClient) GetTorrent(id string) (*transmissionTorrent, error) {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid torrent id: %w", err)
	}

	resp, err := t.doRPC("torrent-get", map[string]interface{}{
		"ids":    []int{intID},
		"fields": torrentFields,
	})
	if err != nil {
		return nil, fmt.Errorf("get torrent: %w", err)
	}

	var result torrentGetResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse get response: %w", err)
	}

	if len(result.Torrents) == 0 {
		return nil, nil
	}
	return &result.Torrents[0], nil
}

// GetAllTorrents returns all torrents known to Transmission.
func (t *TransmissionClient) GetAllTorrents() ([]transmissionTorrent, error) {
	resp, err := t.doRPC("torrent-get", map[string]interface{}{
		"fields": torrentFields,
	})
	if err != nil {
		return nil, fmt.Errorf("get all torrents: %w", err)
	}

	var result torrentGetResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse get-all response: %w", err)
	}

	return result.Torrents, nil
}

// PauseTorrent pauses a torrent by its Transmission ID.
func (t *TransmissionClient) PauseTorrent(id string) error {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid torrent id: %w", err)
	}

	_, err = t.doRPC("torrent-stop", map[string]interface{}{
		"ids": []int{intID},
	})
	return err
}

// ResumeTorrent resumes a torrent by its Transmission ID.
func (t *TransmissionClient) ResumeTorrent(id string) error {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid torrent id: %w", err)
	}

	_, err = t.doRPC("torrent-start", map[string]interface{}{
		"ids": []int{intID},
	})
	return err
}

// RemoveTorrent removes a torrent by its Transmission ID.
// If deleteFiles is true, local data is also deleted.
func (t *TransmissionClient) RemoveTorrent(id string, deleteFiles bool) error {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid torrent id: %w", err)
	}

	_, err = t.doRPC("torrent-remove", map[string]interface{}{
		"ids":               []int{intID},
		"delete-local-data": deleteFiles,
	})
	return err
}

// GetStats returns aggregate stats from all torrents in Transmission.
func (t *TransmissionClient) GetStats() (*TransmissionClientStats, error) {
	torrents, err := t.GetAllTorrents()
	if err != nil {
		return &TransmissionClientStats{}, nil
	}

	stats := &TransmissionClientStats{
		TotalTorrents: len(torrents),
	}
	for _, tor := range torrents {
		switch tor.Status {
		case 4: // downloading
			stats.ActiveTorrents++
		case 0: // stopped/paused
			stats.PausedTorrents++
		case 6: // seeding
			stats.SeedingTorrents++
		}
		stats.DownloadSpeed += tor.RateDownload
		stats.UploadSpeed += tor.RateUpload
		stats.Downloaded += tor.DownloadedEver
		stats.Uploaded += tor.UploadedEver
	}

	return stats, nil
}

// MapStatus converts a Transmission status code to a download status string.
func MapTransmissionStatus(status int) string {
	// Transmission status codes:
	// 0: stopped, 1: check pending, 2: checking,
	// 3: download pending, 4: downloading, 5: seed pending, 6: seeding
	switch status {
	case 0:
		return "paused"
	case 4:
		return "downloading"
	case 6:
		return "seeding"
	default:
		return "queued"
	}
}

// --- Internal RPC execution ---

// doRPC sends a JSON-RPC request to Transmission and returns the arguments
// portion of the response. It handles the X-Transmission-Session-Id header
// retry automatically.
func (t *TransmissionClient) doRPC(method string, args interface{}) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{
		Method:    method,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := t.sendRequest(body)
	if err != nil {
		return nil, err
	}

	// If we get a 409, Transmission sends us a new session ID.
	if resp.StatusCode == http.StatusConflict {
		newID := resp.Header.Get("X-Transmission-Session-Id")
		resp.Body.Close()
		if newID != "" {
			t.mu.Lock()
			t.sessionID = newID
			t.mu.Unlock()

			resp, err = t.sendRequest(body)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("409 conflict but no session ID header")
		}
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if rpcResp.Result != "success" {
		return nil, fmt.Errorf("rpc error: %s", rpcResp.Result)
	}

	return rpcResp.Arguments, nil
}

// sendRequest sends the raw JSON body to the Transmission RPC endpoint.
func (t *TransmissionClient) sendRequest(body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("X-Transmission-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}

	return t.client.Do(req)
}
