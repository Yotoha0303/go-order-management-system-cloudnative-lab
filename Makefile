.DEFAULT_GOAL := help

APP_NAME := go-order-management-system
BIN_DIR := bin
GO ?= go
GOLANGCI_LINT ?= golangci-lint
COMPOSE ?= docker compose
GOOSE ?= goose
PACKAGES := ./...
TEST_FLAGS ?= -count=1
LINT_FLAGS ?=
MIGRATIONS_DIR ?= migrations
DB_HOST ?= 127.0.0.1
DB_PORT ?= 3306
DB_USER ?= root
DB_NAME ?= go_order_management_system
MIGRATION_DSN ?= $(DB_USER):$(MYSQL_PASSWORD)@tcp($(DB_HOST):$(DB_PORT))/$(DB_NAME)?parseTime=true

ifeq ($(OS),Windows_NT)
BINARY := $(BIN_DIR)/$(APP_NAME).exe
else
BINARY := $(BIN_DIR)/$(APP_NAME)
endif

.PHONY: help run dev build clean \
	fmt vet lint tidy mod-download mod-verify \
	test test-verbose test-service test-redis test-all test-race coverage coverage-html \
	check compose-config infra-up infra-down infra-ps infra-logs \
	docker-build docker-up docker-down docker-restart docker-ps docker-logs \
	check-goose check-migration-env migrate-validate migrate-status migrate-up \
	migrate-up-one migrate-up-to migrate-down migrate-down-to migrate-redo \
	migrate-version migrate-create ci

help:
	@echo Usage: make target
	@echo Development:
	@echo   run             Run the API locally
	@echo   dev             Start MySQL/Redis, then run the API
	@echo   build           Build the API binary into $(BIN_DIR)/
	@echo   clean           Remove generated build and coverage files
	@echo Quality:
	@echo   fmt             Format all Go packages
	@echo   vet             Run go vet
	@echo   lint            Run golangci-lint - installation required
	@echo   tidy            Update go.mod and go.sum
	@echo   mod-download    Download Go modules
	@echo   mod-verify      Verify downloaded Go modules
	@echo   check           Run format, module verification, vet, and tests
	@echo Tests:
	@echo   test            Run all tests
	@echo   test-verbose    Run all tests with verbose output
	@echo   test-service    Run MySQL service integration tests
	@echo   test-redis      Run Redis integration tests
	@echo   test-all        Run all tests, including MySQL and Redis integration tests
	@echo   test-race       Run all tests with the race detector
	@echo   coverage        Generate coverage.out
	@echo   coverage-html   Generate coverage.html
	@echo Infrastructure:
	@echo   compose-config  Validate the Docker Compose configuration
	@echo   infra-up        Start MySQL and Redis and wait until healthy
	@echo   infra-down      Stop and remove infrastructure containers
	@echo   infra-ps        Show infrastructure container status
	@echo   infra-logs      Follow MySQL and Redis logs
	@echo Docker:
	@echo   docker-build    Build the application image
	@echo   docker-up       Build and start the complete stack
	@echo   docker-down     Stop and remove the complete stack
	@echo   docker-restart  Restart all services
	@echo   docker-ps       Show all service status
	@echo   docker-logs     Follow all service logs
	@echo Migrations:
	@echo   migrate-validate       Validate migration files without a database
	@echo   migrate-status         Show migration status
	@echo   migrate-up             Apply all pending migrations
	@echo   migrate-up-one         Apply the next migration
	@echo   migrate-up-to          Migrate up to VERSION, e.g. make migrate-up-to VERSION=5
	@echo   migrate-down           Roll back the latest migration
	@echo   migrate-down-to        Roll back to VERSION, e.g. make migrate-down-to VERSION=3
	@echo   migrate-redo           Roll back and re-apply the latest migration
	@echo   migrate-version        Show the current database version
	@echo   migrate-create         Create a SQL migration, e.g. make migrate-create NAME=add_sku

run:
	$(GO) run ./cmd

dev: infra-up run

build:
ifeq ($(OS),Windows_NT)
	@if not exist "$(subst /,\,$(BIN_DIR))" mkdir "$(subst /,\,$(BIN_DIR))"
else
	mkdir -p "$(BIN_DIR)"
endif
	$(GO) build -trimpath -o "$(BINARY)" ./cmd

clean:
ifeq ($(OS),Windows_NT)
	@if exist "$(subst /,\,$(BIN_DIR))" rmdir /S /Q "$(subst /,\,$(BIN_DIR))"
	@if exist coverage.out del /Q coverage.out
	@if exist coverage.html del /Q coverage.html
else
	rm -rf "$(BIN_DIR)" coverage.out coverage.html
endif

fmt:
	$(GO) fmt $(PACKAGES)

vet:
	$(GO) vet $(PACKAGES)

lint:
ifeq ($(OS),Windows_NT)
	@where "$(GOLANGCI_LINT)" || (echo golangci-lint is not installed && exit 1)
else
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { echo "golangci-lint is not installed"; exit 1; }
endif
	$(GOLANGCI_LINT) run $(LINT_FLAGS) $(PACKAGES)

tidy:
	$(GO) mod tidy

mod-download:
	$(GO) mod download

mod-verify:
	$(GO) mod verify

test:
	$(GO) test $(TEST_FLAGS) $(PACKAGES)

test-verbose:
	$(GO) test -v $(TEST_FLAGS) $(PACKAGES)

test-service: export RUN_MYSQL_TEST := 1
test-service:
	$(GO) test -v $(TEST_FLAGS) ./internal/service

test-redis: export RUN_REDIS_TEST := 1
test-redis:
	$(GO) test -v $(TEST_FLAGS) ./internal/bizcache

test-all: test test-service test-redis

test-race:
	$(GO) test -race $(TEST_FLAGS) $(PACKAGES)

coverage:
	$(GO) test $(TEST_FLAGS) -covermode=atomic -coverprofile=coverage.out $(PACKAGES)

coverage-html: coverage
	$(GO) tool cover -html=coverage.out -o coverage.html

check: fmt mod-verify vet test

compose-config:
	$(COMPOSE) config --quiet

infra-up: compose-config
	$(COMPOSE) up -d --wait mysql redis

infra-down:
	$(COMPOSE) down

infra-ps:
	$(COMPOSE) ps

infra-logs:
	$(COMPOSE) logs --follow mysql redis

docker-build: compose-config
	$(COMPOSE) build app

docker-up: compose-config
	$(COMPOSE) up -d --build --wait

docker-down:
	$(COMPOSE) down --remove-orphans

docker-restart: compose-config
	$(COMPOSE) restart

docker-ps:
	$(COMPOSE) ps

docker-logs:
	$(COMPOSE) logs --follow

check-goose:
ifeq ($(OS),Windows_NT)
	@where "$(GOOSE)" || (echo goose is not installed. Run: go install github.com/pressly/goose/v3/cmd/goose@v3.27.1 && exit 1)
else
	@command -v "$(GOOSE)" >/dev/null 2>&1 || { echo "goose is not installed. Run: go install github.com/pressly/goose/v3/cmd/goose@v3.27.1"; exit 1; }
endif

check-migration-env:
ifeq ($(strip $(MYSQL_PASSWORD)),)
	@echo MYSQL_PASSWORD is required. Set it in the environment or pass MYSQL_PASSWORD=... to make. && exit 1
else
	@echo Migration database: $(DB_USER)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)
endif

migrate-validate: check-goose
	$(GOOSE) -dir "$(MIGRATIONS_DIR)" validate

migrate-status: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" status

migrate-up: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" up

migrate-up-one: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" up-by-one

migrate-up-to: check-goose check-migration-env
ifeq ($(strip $(VERSION)),)
	@echo VERSION is required. Example: make migrate-up-to VERSION=5 && exit 1
else
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" up-to "$(VERSION)"
endif

migrate-down: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" down

migrate-down-to: check-goose check-migration-env
ifeq ($(strip $(VERSION)),)
	@echo VERSION is required. Example: make migrate-down-to VERSION=3 && exit 1
else
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" down-to "$(VERSION)"
endif

migrate-redo: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" redo

migrate-version: check-goose check-migration-env
	@$(GOOSE) -dir "$(MIGRATIONS_DIR)" mysql "$(MIGRATION_DSN)" version

migrate-create: check-goose
ifeq ($(strip $(NAME)),)
	@echo NAME is required. Example: make migrate-create NAME=add_product_sku && exit 1
else
	$(GOOSE) -dir "$(MIGRATIONS_DIR)" -s create "$(NAME)" sql
endif

ci:
	$(MAKE) test
	$(MAKE) vet
	$(MAKE) test-race
	$(MAKE) test-redis
	$(MAKE) lint
	$(MAKE) migrate-validate
