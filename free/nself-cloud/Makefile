BINARY   := nself-cloud
PKG      := github.com/nself-org/plugins-pro/paid/nself-cloud
PORT     ?= 3845
DB_URL   ?= postgres://postgres:postgres@localhost:5432/nself

.PHONY: build test run fmt lint clean

## build: compile the plugin binary
build:
	go build -o $(BINARY) .

## test: run unit tests
test:
	go test ./... -v -count=1

## run: build and run the plugin locally
run: build
	PORT=$(PORT) DATABASE_URL=$(DB_URL) ./$(BINARY)

## fmt: format Go source files
fmt:
	gofmt -w .

## lint: run go vet
lint:
	go vet ./...

## clean: remove compiled binary
clean:
	rm -f $(BINARY)

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
