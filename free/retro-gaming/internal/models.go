package internal

import "time"

type ROM struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	RomFilePath          string     `json:"rom_file_path"`
	RomFileSizeBytes     *int64     `json:"rom_file_size_bytes,omitempty"`
	RomFileHash          *string    `json:"rom_file_hash,omitempty"`
	GameTitle            string     `json:"game_title"`
	GameTitleNormalized  string     `json:"game_title_normalized"`
	Platform             string     `json:"platform"`
	Region               *string    `json:"region,omitempty"`
	ReleaseYear          *int       `json:"release_year,omitempty"`
	Genre                *string    `json:"genre,omitempty"`
	Publisher            *string    `json:"publisher,omitempty"`
	Developer            *string    `json:"developer,omitempty"`
	IgdbID               *int       `json:"igdb_id,omitempty"`
	MobyGamesID          *int       `json:"moby_games_id,omitempty"`
	BoxArtURL            *string    `json:"box_art_url,omitempty"`
	BoxArtLocalPath      *string    `json:"box_art_local_path,omitempty"`
	ScreenshotURLs       []string   `json:"screenshot_urls"`
	ScreenshotLocalPaths []string   `json:"screenshot_local_paths"`
	Description          *string    `json:"description,omitempty"`
	DescriptionSource    *string    `json:"description_source,omitempty"`
	RecommendedCore      *string    `json:"recommended_core,omitempty"`
	CoreOverrides        *string    `json:"core_overrides,omitempty"`
	UserRating           *float64   `json:"user_rating,omitempty"`
	PlayCount            int        `json:"play_count"`
	LastPlayedAt         *time.Time `json:"last_played_at,omitempty"`
	Favorite             bool       `json:"favorite"`
	ScanSource           *string    `json:"scan_source,omitempty"`
	AddedByUserID        *string    `json:"added_by_user_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type SaveState struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	RomID                 string     `json:"rom_id"`
	SourceAccountID       string     `json:"source_account_id"`
	Slot                  int        `json:"slot"`
	SaveStateFilePath     string     `json:"save_state_file_path"`
	SaveStateFileSizeBytes *int64    `json:"save_state_file_size_bytes,omitempty"`
	ScreenshotURL         *string    `json:"screenshot_url,omitempty"`
	ScreenshotLocalPath   *string    `json:"screenshot_local_path,omitempty"`
	EmulatorCore          string     `json:"emulator_core"`
	EmulatorVersion       *string    `json:"emulator_version,omitempty"`
	Description           *string    `json:"description,omitempty"`
	PlayTimeSeconds       int        `json:"play_time_seconds"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type PlaySession struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	RomID            string     `json:"rom_id"`
	SourceAccountID  string     `json:"source_account_id"`
	Platform         string     `json:"platform"`
	DeviceID         *string    `json:"device_id,omitempty"`
	EmulatorCore     string     `json:"emulator_core"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	DurationSeconds  *int       `json:"duration_seconds,omitempty"`
	SaveStateID      *string    `json:"save_state_id,omitempty"`
	AutoSaveCreated  bool       `json:"auto_save_created"`
	ControllerType   *string    `json:"controller_type,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type EmulatorCore struct {
	ID                  string    `json:"id"`
	CoreName            string    `json:"core_name"`
	DisplayName         string    `json:"display_name"`
	Platform            string    `json:"platform"`
	CoreWasmPath        *string   `json:"core_wasm_path,omitempty"`
	CoreWasmSizeBytes   *int64    `json:"core_wasm_size_bytes,omitempty"`
	Version             string    `json:"version"`
	License             *string   `json:"license,omitempty"`
	Author              *string   `json:"author,omitempty"`
	HomepageURL         *string   `json:"homepage_url,omitempty"`
	SupportsSaveStates  bool      `json:"supports_save_states"`
	SupportsRewind      bool      `json:"supports_rewind"`
	SupportsFastForward bool      `json:"supports_fast_forward"`
	SupportsCheats      bool      `json:"supports_cheats"`
	DefaultConfig       *string   `json:"default_config,omitempty"`
	IsRecommended       bool      `json:"is_recommended"`
	Priority            int       `json:"priority"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ControllerConfig struct {
	ID                 string    `json:"id"`
	SourceAccountID    string    `json:"source_account_id"`
	UserID             string    `json:"user_id"`
	ConfigName         string    `json:"config_name"`
	Platform           *string   `json:"platform,omitempty"`
	ControllerType     string    `json:"controller_type"`
	ButtonMapping      string    `json:"button_mapping"`
	TouchLayout        *string   `json:"touch_layout,omitempty"`
	AnalogSensitivity  float64   `json:"analog_sensitivity"`
	VibrationEnabled   bool      `json:"vibration_enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CoreInstallation struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	SourceAccountID string    `json:"source_account_id"`
	DeviceID       string     `json:"device_id"`
	DevicePlatform string     `json:"device_platform"`
	CoreName       string     `json:"core_name"`
	CoreVersion    string     `json:"core_version"`
	InstalledAt    time.Time  `json:"installed_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}
