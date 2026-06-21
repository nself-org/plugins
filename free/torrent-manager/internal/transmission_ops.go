package internal

import (
	"encoding/json"
	"fmt"
	"strconv"
)

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
