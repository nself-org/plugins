package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// MLflowClient makes HTTP calls to the underlying MLflow tracking server.
type MLflowClient struct {
	BaseURL    string
	httpClient *http.Client
}

// NewMLflowClient creates an MLflowClient targeting the given base URL.
func NewMLflowClient(baseURL string) *MLflowClient {
	return &MLflowClient{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// get performs a GET request to the MLflow REST API and decodes the JSON response.
func (c *MLflowClient) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}
	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request to %s: %w", u.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mlflow returned status %d for %s", resp.StatusCode, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response from %s: %w", path, err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Experiments
// --------------------------------------------------------------------------

// ExperimentInfo mirrors the MLflow experiment object returned by the REST API.
type ExperimentInfo struct {
	ExperimentID   string            `json:"experiment_id"`
	Name           string            `json:"name"`
	ArtifactLocation string          `json:"artifact_location"`
	LifecycleStage string            `json:"lifecycle_stage"`
	LastUpdateTime int64             `json:"last_update_time"`
	CreationTime   int64             `json:"creation_time"`
	Tags           []ExperimentTag   `json:"tags"`
}

// ExperimentTag is a key-value tag on an experiment.
type ExperimentTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type searchExperimentsResponse struct {
	Experiments   []ExperimentInfo `json:"experiments"`
	NextPageToken string           `json:"next_page_token"`
}

type getExperimentResponse struct {
	Experiment ExperimentInfo `json:"experiment"`
}

// SearchExperiments returns all experiments from the MLflow server.
func (c *MLflowClient) SearchExperiments(ctx context.Context) ([]ExperimentInfo, error) {
	var result searchExperimentsResponse
	if err := c.get(ctx, "/api/2.0/mlflow/experiments/search", nil, &result); err != nil {
		return nil, fmt.Errorf("searching experiments: %w", err)
	}
	return result.Experiments, nil
}

// GetExperiment returns the experiment with the given ID.
func (c *MLflowClient) GetExperiment(ctx context.Context, id string) (ExperimentInfo, error) {
	params := url.Values{"experiment_id": []string{id}}
	var result getExperimentResponse
	if err := c.get(ctx, "/api/2.0/mlflow/experiments/get", params, &result); err != nil {
		return ExperimentInfo{}, fmt.Errorf("getting experiment %q: %w", id, err)
	}
	return result.Experiment, nil
}

// --------------------------------------------------------------------------
// Runs
// --------------------------------------------------------------------------

// RunInfo mirrors the MLflow run info object.
type RunInfo struct {
	RunID          string `json:"run_id"`
	ExperimentID   string `json:"experiment_id"`
	RunName        string `json:"run_name"`
	Status         string `json:"status"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	ArtifactURI    string `json:"artifact_uri"`
	LifecycleStage string `json:"lifecycle_stage"`
}

// RunData holds the metrics, params, and tags of a run.
type RunData struct {
	Metrics []Metric   `json:"metrics"`
	Params  []RunParam `json:"params"`
	Tags    []RunTag   `json:"tags"`
}

// Metric is a single metric value within a run.
type Metric struct {
	Key       string  `json:"key"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
	Step      int64   `json:"step"`
}

// RunParam is a single parameter value within a run.
type RunParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RunTag is a key-value tag on a run.
type RunTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Run combines run info and data.
type Run struct {
	Info RunInfo `json:"info"`
	Data RunData `json:"data"`
}

type searchRunsRequest struct {
	ExperimentIDs []string `json:"experiment_ids"`
}

type searchRunsResponse struct {
	Runs          []Run  `json:"runs"`
	NextPageToken string `json:"next_page_token"`
}

type getRunResponse struct {
	Run Run `json:"run"`
}

// SearchRuns returns all runs for the given experiment ID.
func (c *MLflowClient) SearchRuns(ctx context.Context, experimentID string) ([]Run, error) {
	params := url.Values{"experiment_ids": []string{experimentID}}
	var result searchRunsResponse
	if err := c.get(ctx, "/api/2.0/mlflow/runs/search", params, &result); err != nil {
		return nil, fmt.Errorf("searching runs for experiment %q: %w", experimentID, err)
	}
	return result.Runs, nil
}

// GetRun returns the run with the given run ID.
func (c *MLflowClient) GetRun(ctx context.Context, runID string) (Run, error) {
	params := url.Values{"run_id": []string{runID}}
	var result getRunResponse
	if err := c.get(ctx, "/api/2.0/mlflow/runs/get", params, &result); err != nil {
		return Run{}, fmt.Errorf("getting run %q: %w", runID, err)
	}
	return result.Run, nil
}

// --------------------------------------------------------------------------
// Registered models
// --------------------------------------------------------------------------

// RegisteredModel mirrors the MLflow registered model object.
type RegisteredModel struct {
	Name              string              `json:"name"`
	CreationTimestamp int64               `json:"creation_timestamp"`
	LastUpdatedTimestamp int64            `json:"last_updated_timestamp"`
	Description       string              `json:"description"`
	LatestVersions    []ModelVersion      `json:"latest_versions"`
	Tags              []RegisteredModelTag `json:"tags"`
}

// ModelVersion is a version entry within a registered model.
type ModelVersion struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	CreationTimestamp int64 `json:"creation_timestamp"`
	LastUpdatedTimestamp int64 `json:"last_updated_timestamp"`
	CurrentStage     string `json:"current_stage"`
	Description      string `json:"description"`
	Source           string `json:"source"`
	RunID            string `json:"run_id"`
	Status           string `json:"status"`
}

// RegisteredModelTag is a key-value tag on a registered model.
type RegisteredModelTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type searchRegisteredModelsResponse struct {
	RegisteredModels []RegisteredModel `json:"registered_models"`
	NextPageToken    string            `json:"next_page_token"`
}

type getRegisteredModelResponse struct {
	RegisteredModel RegisteredModel `json:"registered_model"`
}

// SearchRegisteredModels returns all registered models from the MLflow model registry.
func (c *MLflowClient) SearchRegisteredModels(ctx context.Context) ([]RegisteredModel, error) {
	var result searchRegisteredModelsResponse
	if err := c.get(ctx, "/api/2.0/mlflow/registered-models/search", nil, &result); err != nil {
		return nil, fmt.Errorf("searching registered models: %w", err)
	}
	return result.RegisteredModels, nil
}

// GetRegisteredModel returns the registered model with the given name.
func (c *MLflowClient) GetRegisteredModel(ctx context.Context, name string) (RegisteredModel, error) {
	params := url.Values{"name": []string{name}}
	var result getRegisteredModelResponse
	if err := c.get(ctx, "/api/2.0/mlflow/registered-models/get", params, &result); err != nil {
		return RegisteredModel{}, fmt.Errorf("getting registered model %q: %w", name, err)
	}
	return result.RegisteredModel, nil
}
