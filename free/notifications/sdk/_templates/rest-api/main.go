package main

import (
	"encoding/json"
	"net/http"
	"time"

	sdkconfig  "github.com/nself-org/cli/sdk/go/config"
	sdklogger  "github.com/nself-org/cli/sdk/go/logger"
	sdkmetrics "github.com/nself-org/cli/sdk/go/metrics"
	sdkplugin  "github.com/nself-org/cli/sdk/go/plugin"
	sdkserver  "github.com/nself-org/cli/sdk/go/server"
)

const (
	pluginName    = "rest-api-plugin"
	pluginVersion = "0.1.0"
	pluginPort    = 3900
)

func main() {
	log := sdklogger.New(pluginName, pluginVersion)
	cfg := sdkconfig.MustLoad()
	metrics := sdkmetrics.NewRegistry(pluginName)
	srv := sdkserver.New(sdkserver.Config{
		Port:    pluginPort,
		Logger:  log,
		Metrics: metrics,
	})

	// Mount your handlers
	srv.Handle("/rest-api-plugin/hello", http.HandlerFunc(handleHello))
	srv.Handle("/rest-api-plugin/items", http.HandlerFunc(handleItems))

	_ = cfg // use cfg.GetString("REST_API_PLUGIN_SECRET") for env vars

	sdkplugin.Run(srv, sdkplugin.Info{
		Name:    pluginName,
		Version: pluginVersion,
		Logger:  log,
	})
}

func handleHello(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"plugin":    pluginName,
		"timestamp": time.Now().Unix(),
	})
}

func handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"created": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
