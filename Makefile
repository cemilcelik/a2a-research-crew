# a2a-research-crew geliştirme komutları.
# Not: Uzun süren Docker komutları arka planda çalıştırılır; Go doğrulaması ana akıştadır.

GO ?= go
BIN_DIR := bin

AGENTS := orchestrator market-scout competitor-analyst report-writer cli

.PHONY: all
all: fmt vet test build

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: fmt-check
fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then echo "gofmt gerekli:"; echo "$$files"; exit 1; fi

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: test-race
test-race:
	$(GO) test -race ./...

.PHONY: build
build: $(AGENTS:%=$(BIN_DIR)/%)

$(BIN_DIR)/%:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $@ ./cmd/$*

.PHONY: run-orchestrator
run-orchestrator:
	$(GO) run ./cmd/orchestrator

.PHONY: run-market-scout
run-market-scout:
	$(GO) run ./cmd/market-scout

.PHONY: run-competitor-analyst
run-competitor-analyst:
	$(GO) run ./cmd/competitor-analyst

.PHONY: run-report-writer
run-report-writer:
	$(GO) run ./cmd/report-writer

.PHONY: run-cli
run-cli:
	$(GO) run ./cmd/cli

.PHONY: docker-build
docker-build:
	docker compose build

.PHONY: up
up:
	docker compose up -d

.PHONY: up-minio
up-minio:
	docker compose --profile minio up -d

.PHONY: down
down:
	docker compose down

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
