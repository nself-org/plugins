module github.com/nself-org/plugins-pro/paid/nself-saas-gateway

go 1.25.0

toolchain go1.26.6

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/go-chi/chi/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.12.3
	github.com/nself-org/plugins-pro/paid/shared v0.0.0
)

require golang.org/x/crypto v0.53.0 // indirect

replace github.com/nself-org/plugins-pro/paid/shared => ../shared
