package model

//go:generate go run ../../tools/schemagen

// PipelineConfig is the authored ci.pipeline-config/v1 document.
type PipelineConfig struct {
	Version   int                    `json:"version"`
	Support   []string               `json:"support,omitempty"`
	Detect    *DetectConfig          `json:"detect,omitempty"`
	Presets   []Preset               `json:"presets,omitempty"`
	Services  map[string]Service     `json:"services,omitempty"`
	Checks    map[string]CheckDef    `json:"checks,omitempty"`
	Jobs      map[string]Job         `json:"jobs,omitempty"`
	Pipelines map[string]PipelineDef `json:"pipelines,omitempty"`
	Policy    map[string]any         `json:"policy,omitempty"`
	Cache     *CacheConfig           `json:"cache,omitempty"`
}

// DetectConfig controls ecosystem detection.
type DetectConfig struct {
	Disable []string `json:"disable,omitempty"`
	Roots   []string `json:"roots,omitempty"`
}

// Service is a named nSelf service dependency.
type Service struct {
	Component string `json:"component"`
	Version   string `json:"version,omitempty"`
}

// CheckDef configures a named check.
type CheckDef struct {
	Required      *bool    `json:"required,omitempty"`
	CoverageFloor *int     `json:"coverage_floor,omitempty"`
	Rules         []string `json:"rules,omitempty"`
}

// Job is an authored job definition. The final two fields are internal only.
type Job struct {
	Kind             JobKind           `json:"kind"`
	Run              []string          `json:"run"`
	Workdir          string            `json:"workdir,omitempty"`
	Deps             []string          `json:"deps,omitempty"`
	NeedsServices    []string          `json:"needs_services,omitempty"`
	Matrix           *JobMatrix        `json:"matrix,omitempty"`
	Required         *bool             `json:"required,omitempty"`
	AllowFailure     bool              `json:"allow_failure,omitempty"`
	Idempotent       *bool             `json:"idempotent,omitempty"`
	Timeout          string            `json:"timeout,omitempty"`
	Retry            *RetryConfig      `json:"retry,omitempty"`
	Priority         Priority          `json:"priority,omitempty"`
	Path             JobPath           `json:"path,omitempty"`
	When             *WhenConfig       `json:"when,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Secrets          []string          `json:"secrets,omitempty"`
	Isolation        Isolation         `json:"isolation,omitempty"`
	Container        *ContainerConfig  `json:"container,omitempty"`
	Artifacts        []ArtifactDecl    `json:"artifacts,omitempty"`
	Cache            *JobCacheConfig   `json:"cache,omitempty"`
	Requirements     *Requirements     `json:"requires,omitempty"`
	RunnerCredential string            `json:"-"`
	RemoteOnly       bool              `json:"-"`
}

// JobMatrix limits the target platforms.
type JobMatrix struct {
	Platform any `json:"platform"`
}

// RetryConfig bounds infrastructure retries.
type RetryConfig struct {
	InfraMax int `json:"infra_max,omitempty"`
}

// WhenConfig selects a job by event or environment.
type WhenConfig struct {
	Branches   []string  `json:"branches,omitempty"`
	Paths      []string  `json:"paths,omitempty"`
	Triggers   []Trigger `json:"triggers,omitempty"`
	EnvPresent []string  `json:"env_present,omitempty"`
}

// ContainerConfig pins a container image.
type ContainerConfig struct {
	Image string `json:"image"`
}

// PipelineDef selects jobs for a named pipeline.
type PipelineDef struct {
	Triggers  []Trigger `json:"triggers"`
	Schedule  string    `json:"schedule,omitempty"`
	MaxAge    string    `json:"max_age,omitempty"`
	Jobs      []string  `json:"jobs,omitempty"`
	DrillsDir string    `json:"drills_dir,omitempty"`
}

// CacheConfig controls the project cache.
type CacheConfig struct {
	Disable bool `json:"disable,omitempty"`
}

// JobCacheConfig controls a job's cache keys and paths.
type JobCacheConfig struct {
	Disable bool     `json:"disable,omitempty"`
	Keys    []string `json:"keys,omitempty"`
	Paths   []string `json:"paths,omitempty"`
}

// ArtifactDecl names an output that must be committed before success.
type ArtifactDecl struct {
	Path       string             `json:"path"`
	Name       string             `json:"name,omitempty"`
	Kind       ArtifactKind       `json:"kind,omitempty"`
	Retention  string             `json:"retention,omitempty"`
	Visibility ArtifactVisibility `json:"visibility,omitempty"`
	Image      *ArtifactImage     `json:"image,omitempty"`
}

// ArtifactImage identifies an image output.
type ArtifactImage struct {
	Ref string `json:"ref"`
}

// Requirements narrow eligible runners.
type Requirements struct {
	Labels       []string `json:"labels,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Accelerators []string `json:"accelerators,omitempty"`
	CPU          int      `json:"cpu,omitempty"`
	MemMB        int      `json:"mem_mb,omitempty"`
}
