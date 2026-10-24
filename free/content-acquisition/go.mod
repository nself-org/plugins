module github.com/nself-org/nself-content-acquisition

go 1.26.4

require (
	github.com/go-chi/chi/v5 v5.4.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/nself-org/plugin-sdk v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/nself-org/cli/sdk/go/v2 v2.0.0
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/nself-org/plugin-sdk => ./third_party/sdk

replace github.com/nself-org/cli/sdk/go/v2 => ./third_party/cli-sdk-go
