GO ?= go
BIN ?= bin/ta$(shell $(GO) env GOEXE)
ARGS ?= --help

.PHONY: help build build-manual install run test race vet bench bench-plan bench-downloads
help:
	@echo "make install      Install ta into GOBIN (or GOPATH/bin)"
	@echo "make build        Build $(BIN)"
	@echo "make run ARGS='archive --all --types channels --format both'"
	@echo "make test race vet bench"
	@echo "make bench-plan bench-downloads  Planning and controlled ta/tdl-core benchmarks"
	@echo "ta sync --everything --every 6h --format both  Recurring incremental backups"

build:
	$(GO) build -trimpath -o "$(BIN)" ./cmd/ta

build-manual: build

install:
	$(GO) install -trimpath ./cmd/ta

run: build
	"./$(BIN)" $(ARGS)

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

bench:
	$(GO) test ./internal/archive -run '^$$' -bench . -benchmem

bench-plan:
	$(GO) test ./internal/archive -run '^$$' -bench '^BenchmarkPlanBatch$$' -benchmem -benchtime=5x -count=5

bench-downloads:
	$(GO) test ./internal/telegram -run '^$$' -bench '^BenchmarkDownloadEngines$$' -benchmem -benchtime=1x -count=5
