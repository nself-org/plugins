module github.com/nself-org/nself-cdc

go 1.26.0

toolchain go1.26.6

require (
	github.com/jackc/pgx/v5 v5.9.2
	github.com/rabbitmq/amqp091-go v1.13.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/nself-org/cli/sdk/go/v2 v2.0.0
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/nself-org/cli/sdk/go/v2 => ./third_party/cli-sdk-go
