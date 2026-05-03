package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewMLflowClient_BaseURL verifies that NewMLflowClient stores the base URL.
func TestNewMLflowClient_BaseURL(t *testing.T) {
	c := NewMLflowClient("http://mlflow.example.com")
	if c.BaseURL != "http://mlflow.example.com" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, "http://mlflow.example.com")
	}
}

// TestNewMLflowClient_NotNil verifies that the constructor returns a non-nil client.
func TestNewMLflowClient_NotNil(t *testing.T) {
	c := NewMLflowClient("http://localhost:5000")
	if c == nil {
		t.Fatal("NewMLflowClient returned nil")
	}
}

// TestSearchExperiments_HappyPath verifies that SearchExperiments decodes a valid
// response and returns experiments.
func TestSearchExperiments_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/experiments/search" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"experiments": []map[string]interface{}{
				{"experiment_id": "1", "name": "my-exp", "lifecycle_stage": "active"},
			},
		})
	}))
	defer srv.Close()

	c := NewMLflowClient(srv.URL)
	exps, err := c.SearchExperiments(context.Background())
	if err != nil {
		t.Fatalf("SearchExperiments error: %v", err)
	}
	if len(exps) != 1 {
		t.Fatalf("len(exps) = %d, want 1", len(exps))
	}
	if exps[0].Name != "my-exp" {
		t.Errorf("Name = %q, want %q", exps[0].Name, "my-exp")
	}
}

// TestSearchExperiments_ServerError verifies that a non-200 response returns an error.
func TestSearchExperiments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewMLflowClient(srv.URL)
	_, err := c.SearchExperiments(context.Background())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

// TestGetExperiment_HappyPath verifies GetExperiment decodes a single experiment.
func TestGetExperiment_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/experiments/get" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"experiment": map[string]interface{}{
				"experiment_id": "42",
				"name":          "test-experiment",
				"lifecycle_stage": "active",
			},
		})
	}))
	defer srv.Close()

	c := NewMLflowClient(srv.URL)
	exp, err := c.GetExperiment(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetExperiment error: %v", err)
	}
	if exp.ExperimentID != "42" {
		t.Errorf("ExperimentID = %q, want %q", exp.ExperimentID, "42")
	}
}

// TestGetExperiment_NotFound verifies that a 404 from the server propagates as an error.
func TestGetExperiment_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewMLflowClient(srv.URL)
	_, err := c.GetExperiment(context.Background(), "999")
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}
