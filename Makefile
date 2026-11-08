GO ?= go
BIN ?= bin/ta
ARGS ?= --help

.PHONY: help build build-manual run test race vet bench
help:
	@echo "make build        Build bin/ta"
	@echo "make run ARGS='archive --all --types channels --format both'"
	@echo "make test race vet bench"
	@echo "./bin/ta sync --everything --every 6h --format both  Recurring incremental backups"

build:
	$(GO) build -trimpath -o $(BIN) .

build-manual: build

run: build
	./$(BIN) $(ARGS)

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

bench:
	$(GO) test ./internal/archive -run '^$$' -bench . -benchmem
