module github.com/nself-org/nself-auth-enterprise

go 1.26.0

toolchain go1.26.6

require (
	github.com/jackc/pgx/v5 v5.9.2
	github.com/nself-org/plugins/free/shared-utils v0.0.0
	github.com/pquerna/otp v1.4.0
	golang.org/x/crypto v0.56.0
)

replace github.com/nself-org/plugins/free/shared-utils => ./third_party/shared-utils

require (
	github.com/boombuler/barcode v1.0.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/nself-org/cli/sdk/go/v2 v2.0.0
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/nself-org/cli/sdk/go/v2 => ./third_party/cli-sdk-go
