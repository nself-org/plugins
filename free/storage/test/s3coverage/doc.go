// Package s3coverage is the S3 coverage suite for plugin-storage.
//
// Purpose: prove that an S3 endpoint (chrislusf/seaweedfs, pgsty/minio, or any other) supports every operation
// nSelf apps and plugins use, and that plugin-storage stores bytes in S3 and only metadata in Postgres.
// Inputs: S3COV_ENDPOINT, S3COV_ACCESS_KEY, S3COV_SECRET_KEY, S3COV_REGION (default us-east-1), S3COV_DB_URL.
// Outputs: Go test results (TestOp_<Operation> and TestPlugin_*).
// Constraints:
//   - The tests carry the build tag s3coverage, so a plain `go test ./...` compiles nothing here.
//   - Run it with run.sh IMAGE (seaweedfs|minio), which starts the pinned image and a Postgres in docker.
//   - Credentials come from the environment only; this package holds no secret.
package s3coverage
