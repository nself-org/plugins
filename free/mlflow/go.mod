module github.com/nself-org/nself-mlflow

go 1.23.0

require (
	github.com/go-chi/chi/v5 v5.2.1
	github.com/nself-org/plugin-sdk v0.0.0
)

replace github.com/nself-org/plugin-sdk => ../../../cli/sdk/go
