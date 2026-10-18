module github.com/nself-org/nself-event-bus

go 1.26.0

toolchain go1.26.6

require (
	github.com/jackc/pgx/v5 v5.9.2
	github.com/nats-io/nats-server/v2 v2.11.15
	github.com/nats-io/nats.go v1.49.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.6.0-default-no-op // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/minio/highwayhash v1.0.4-0.20251030100505-070ab1a87a76 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nself-org/cli/sdk/go/v2 v2.0.0
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/nself-org/cli/sdk/go/v2 => ./third_party/cli-sdk-go
