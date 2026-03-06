.PHONY: build run test clean docker-build docker-up docker-down dev \
       hub-build hub-push hub-prod hub-down \
       deploy deploy-compose ci ci-local

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
BINARY_NAME=server

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/server

# Run the application (loads .env file)
run: build
	@export $$(grep -v '^#' .env | xargs) && ./$(BINARY_NAME)

# Run tests
test:
	$(GOTEST) -v ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Build Docker image
docker-build:
	docker build -t meetwhen .

# Start development environment (local with SQLite)
dev: build
	@export $$(grep -v '^#' .env | xargs) && ./$(BINARY_NAME)

# Start development environment with Docker (Postgres + Mailhog)
dev-docker:
	docker-compose --profile dev up -d

# Start production environment
prod:
	docker-compose --profile prod up -d

# Stop all containers
docker-down:
	docker-compose --profile dev --profile prod down

# View logs
logs:
	docker-compose logs -f app

# Run database migrations manually
migrate:
	docker-compose exec app ./server migrate

# Create a backup of the database
backup:
	docker-compose exec db pg_dump -U meetwhen meetwhen > backup_$$(date +%Y%m%d_%H%M%S).sql

# Restore database from backup
restore:
	@echo "Usage: cat backup.sql | docker-compose exec -T db psql -U meetwhen meetwhen"

# Docker Hub workaround (when GHCR/Actions unavailable)
DOCKER_HUB_USER=ceesaxp
DOCKER_HUB_IMAGE=$(DOCKER_HUB_USER)/meet-when
VERSION?=$(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)

# Build multi-arch image and push to Docker Hub
hub-build:
	docker buildx build --builder mybuilder \
		--platform linux/amd64,linux/arm64 \
		-t $(DOCKER_HUB_IMAGE):$(VERSION) \
		-t $(DOCKER_HUB_IMAGE):latest \
		--push .

# Push existing local image to Docker Hub (single arch)
hub-push:
	docker tag meetwhen $(DOCKER_HUB_IMAGE):$(VERSION)
	docker tag meetwhen $(DOCKER_HUB_IMAGE):latest
	docker push $(DOCKER_HUB_IMAGE):$(VERSION)
	docker push $(DOCKER_HUB_IMAGE):latest

# Start production using Docker Hub image
hub-prod:
	IMAGE_TAG=$(VERSION) docker compose -f docker-compose-hub.yml --profile prod up -d

# Stop Docker Hub production containers
hub-down:
	docker compose -f docker-compose-hub.yml --profile prod down

# Remote deployment via SSH (mirrors GHA deploy workflow)
DEPLOY_KEY?=~/.ssh/mw-app_key
DEPLOY_USER?=mw-app
DEPLOY_HOST?=
DEPLOY_PATH?=/opt/meet-when/mw-prod

# Deploy: pull latest image and restart on remote host
deploy:
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required"; exit 1; fi
	ssh -i $(DEPLOY_KEY) $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		set -e && \
		docker pull $(DOCKER_HUB_IMAGE):$(VERSION) && \
		cd $(DEPLOY_PATH) && \
		IMAGE_TAG=$(VERSION) docker compose -f docker-compose-hub.yml --profile prod down && \
		IMAGE_TAG=$(VERSION) docker compose -f docker-compose-hub.yml --profile prod up -d --remove-orphans && \
		docker system prune -f && \
		echo "Deployed $(DOCKER_HUB_IMAGE):$(VERSION) successfully"'

# Deploy with updated docker-compose file: copy compose file, then deploy
deploy-compose:
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required"; exit 1; fi
	scp -i $(DEPLOY_KEY) docker-compose-hub.yml $(DEPLOY_USER)@$(DEPLOY_HOST):$(DEPLOY_PATH)/docker-compose-hub.yml
	$(MAKE) deploy

# CI: run GitHub Actions workflow locally via act
ci:
	act push --container-architecture linux/amd64

# CI without Docker: test + build + push (no act required)
ci-local: test hub-build

# Development helpers
fmt:
	$(GOCMD) fmt ./...

lint:
	golangci-lint run

# Install development dependencies
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy
