.PHONY: proto generate build-hive build-drone docker-build docker-up docker-down test clean

# Generate Go code from protobuf
proto:
	@echo "Generating protobuf Go code..."
	@mkdir -p internal/proto
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/hive.proto

# Generate all code
generate: proto

# Build the Hive server binary
build-hive:
	@echo "Building Hive server..."
	@go build -o bin/hive-server ./cmd/hive-server

# Build the Drone client binary
build-drone:
	@echo "Building Drone client..."
	@go build -o bin/drone-client ./cmd/drone-client

# Build all binaries
build: build-hive build-drone

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	@docker-compose build

# Start Docker services
docker-up:
	@echo "Starting Docker services..."
	@docker-compose up -d

# Stop Docker services
docker-down:
	@echo "Stopping Docker services..."
	@docker-compose down

# Run tests
test:
	@echo "Running tests..."
	@go test ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf internal/proto/*.pb.go
	@rm -rf internal/proto/*_grpc.pb.go

# Install dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

