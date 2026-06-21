package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
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
