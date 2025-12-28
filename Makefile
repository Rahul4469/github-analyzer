# ===========================================
# GitHub Analyzer - Makefile
# ===========================================
# Usage: make <target>
# Run 'make help' to see all available targets

.PHONY: help build run dev clean test migrate-up migrate-down migrate-status migrate-create deps lint

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=github-analyzer
MAIN_PATH=./cmd/server
BUILD_DIR=./bin
MIGRATIONS_DIR=./migrations

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

# Colors for terminal output
COLOR_RESET=\033[0m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

## help: Show this help message
help:
	@echo "$(COLOR_BLUE)GitHub Analyzer - Available Commands$(COLOR_RESET)"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application binary
build:
	@echo "$(COLOR_GREEN)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

## run: Build and run the application
run: build
	@echo "$(COLOR_GREEN)Starting $(BINARY_NAME)...$(COLOR_RESET)"
	$(BUILD_DIR)/$(BINARY_NAME)

## dev: Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	@echo "$(COLOR_GREEN)Starting development server with hot reload...$(COLOR_RESET)"
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

## clean: Remove build artifacts
clean:
	@echo "$(COLOR_YELLOW)Cleaning build artifacts...$(COLOR_RESET)"
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "$(COLOR_GREEN)Clean complete$(COLOR_RESET)"

## test: Run all tests
test:
	@echo "$(COLOR_GREEN)Running tests...$(COLOR_RESET)"
	$(GOTEST) -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "$(COLOR_GREEN)Running tests with coverage...$(COLOR_RESET)"
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(COLOR_GREEN)Coverage report: coverage.html$(COLOR_RESET)"

## deps: Download and tidy dependencies
deps:
	@echo "$(COLOR_GREEN)Downloading dependencies...$(COLOR_RESET)"
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Dependencies updated$(COLOR_RESET)"

## lint: Run linter (requires golangci-lint)
lint:
	@echo "$(COLOR_GREEN)Running linter...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# ===========================================
# Database Migration Commands
# ===========================================
# Requires: DATABASE_URL environment variable

## migrate-up: Run all pending migrations
migrate-up:
	@echo "$(COLOR_GREEN)Running migrations...$(COLOR_RESET)"
	goose -dir $(MIGRATIONS_DIR) postgres "host=localhost port=5432 user=rahul password=junglebook dbname=gitanalyze sslmode=disable" up
	@echo "$(COLOR_GREEN)Migrations complete$(COLOR_RESET)"

## migrate-down: Rollback the last migration
migrate-down:
	@echo "$(COLOR_YELLOW)Rolling back last migration...$(COLOR_RESET)"
	goose -dir $(MIGRATIONS_DIR) postgres "host=localhost port=5432 user=rahul password=junglebook dbname=gitanalyze sslmode=disable" down

## migrate-status: Show migration status
migrate-status:
	@echo "$(COLOR_BLUE)Migration status:$(COLOR_RESET)"
	goose -dir $(MIGRATIONS_DIR) postgres "host=localhost port=5432 user=rahul password=junglebook dbname=gitanalyze sslmode=disable" status

## migrate-create: Create a new migration file (usage: make migrate-create name=create_users)
migrate-create:
	@if [ -z "$(name)" ]; then \
		echo "$(COLOR_YELLOW)Usage: make migrate-create name=migration_name$(COLOR_RESET)"; \
		exit 1; \
	fi
	@echo "$(COLOR_GREEN)Creating migration: $(name)$(COLOR_RESET)"
	@mkdir -p $(MIGRATIONS_DIR)
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

## migrate-reset: Drop all tables and re-run migrations (DESTRUCTIVE!)
migrate-reset:
	@echo "$(COLOR_YELLOW)WARNING: This will delete all data!$(COLOR_RESET)"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	goose -dir $(MIGRATIONS_DIR) postgres "host=localhost port=5432 user=rahul password=junglebook dbname=gitanalyze sslmode=disable" reset
	goose -dir $(MIGRATIONS_DIR) postgres "host=localhost port=5432 user=rahul password=junglebook dbname=gitanalyze sslmode=disable" up

# ===========================================
# Database Setup Commands
# ===========================================

## db-create: Create the database (requires psql)
db-create:
	@echo "$(COLOR_GREEN)Creating database...$(COLOR_RESET)"
	createdb -U postgres gitanalyze || echo "Database may already exist"

## db-drop: Drop the database (DESTRUCTIVE!)
db-drop:
	@echo "$(COLOR_YELLOW)WARNING: This will delete the database!$(COLOR_RESET)"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	dropdb -U postgres github_analyzer

## db-setup: Create database and run migrations
db-setup: db-create migrate-up
	@echo "$(COLOR_GREEN)Database setup complete$(COLOR_RESET)"

# ===========================================
# Development Setup
# ===========================================

## setup: Complete development environment setup
setup: deps db-setup
	@echo "$(COLOR_GREEN)Setup complete!$(COLOR_RESET)"
	@echo "$(COLOR_BLUE)Next steps:$(COLOR_RESET)"
	@echo "  1. Copy .env.example to .env and configure"
	@echo "  2. Run 'make dev' to start the server"

## install-tools: Install required development tools
install-tools:
	@echo "$(COLOR_GREEN)Installing development tools...$(COLOR_RESET)"
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	@echo "$(COLOR_GREEN)Tools installed$(COLOR_RESET)"