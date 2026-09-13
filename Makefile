MAKEFLAGS       += --no-print-directory

VERSION         ?= v1.0.0
BIN_DIR         ?= bin
PACKAGES        ?= ./...

GO_LDFLAGS_PART := -s -w
GO_LDFLAGS_PART += -X "github.com/go-sdk/core/osx.iVersion=$(VERSION)"
GO_LDFLAGS      := -ldflags '$(GO_LDFLAGS_PART)'

GORM_CLI_VERSION ?= v0.2.4 # https://github.com/go-gorm/cli

.PHONY: tidy
tidy:					##@ Tidy go.mod and go.sum.
	@go mod tidy

.PHONY: prepare
prepare:				##@ Install code generation tools.
	@go install gorm.io/cli/gorm@$(GORM_CLI_VERSION)

.PHONY: generate
generate:				##@ Generate tests/modelg from tests/models.
	@gorm gen -i tests/models && echo "done.";

.PHONY: lint
lint: tidy				##@ Lint all packages.
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m && \
		echo "done."; \
	else \
		echo "golangci-lint is not installed. Please install it from https://github.com/golangci/golangci-lint"; \
		exit 1; \
	fi

.PHONY: test
test: tidy				##@ Test all packages.
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --format-icons text -- -race -count 1 -failfast -v $(PACKAGES); \
	else \
		go test -race -count 1 -failfast -v $(PACKAGES); \
	fi

.PHONY: local-test
local-test:				##@ Run tests/db against local docker databases.
	@status=0; \
	docker compose --profile mysql --profile mariadb --profile postgres up --detach --wait --wait-timeout 180 || status=$$?; \
	if [ $$status -eq 0 ]; then \
		TEST_MYSQL_DSN="root:12345678@tcp(127.0.0.1:3001)/test" \
		TEST_MARIADB_DSN="root:12345678@tcp(127.0.0.1:3002)/test" \
		TEST_POSTGRES_DSN="postgres://postgres:12345678@127.0.0.1:3003/test?sslmode=disable" \
		$(MAKE) test PACKAGES=./tests/db/... || status=$$?; \
	fi; \
	docker compose --profile "*" down; \
	exit $$status

.PHONY: build
build:					##@ Build tests/db into bin.
	@mkdir -p $(BIN_DIR)
	@go build $(GO_LDFLAGS) -o $(BIN_DIR)/db ./tests/db

.PHONY: run
run: build				##@ Run tests/db example.
	@$(BIN_DIR)/db


.PHONY: help
help:					##@ (Default) Show help.
	@printf "\nUsage: make <command>\n"
	@grep -F -h "##@" $(MAKEFILE_LIST) | grep -F -v grep -F | sed -e 's/\\$$//' | awk 'BEGIN {FS = ":*[[:space:]]*##@[[:space:]]*"}; \
	{ \
		if($$2 == "") \
			pass; \
		else if($$0 ~ /^#/) \
			printf "\n%s\n", $$2; \
		else if($$1 == "") \
			printf "     %-20s%s\n", "", $$2; \
		else \
			printf "\n    \033[34m%-20s\033[0m %s\n", $$1, $$2; \
	}'
	@printf "\n"

.DEFAULT_GOAL := help
