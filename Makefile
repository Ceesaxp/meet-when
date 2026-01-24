.PHONY: build run test clean docker-build docker-up docker-down dev

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
BINARY_NAME=server

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/server

# Run the application
run: build
	./$(BINARY_NAME)

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

# Start development environment
dev:
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

# Development helpers
fmt:
	$(GOCMD) fmt ./...

lint:
	golangci-lint run

# Install development dependencies
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy
