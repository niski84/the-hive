# Setup Guide

## Prerequisites

1. **Go 1.24+**: Install from https://go.dev/dl/
2. **Protobuf Compiler**: 
   ```bash
   # Ubuntu/Debian
   sudo apt-get install protobuf-compiler
   
   # macOS
   brew install protobuf
   ```
3. **Protobuf Go Plugins**:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```
4. **Docker & Docker Compose**: For running Qdrant and deployment

## Initial Setup

1. **Install Go dependencies**:
   ```bash
   go mod download
   go mod tidy
   ```

2. **Generate Protobuf code**:
   ```bash
   make proto
   ```
   This will generate the gRPC code in `backend/internal/proto/`

3. **Build the binaries**:
   ```bash
   make build
   ```
   This creates:
   - `bin/hive-server` - The Hive server binary
   - `bin/drone-client` - The Drone client binary

## Running Locally

### Start Qdrant (Vector Database)

```bash
docker run -d -p 6333:6333 -p 6334:6334 --name qdrant qdrant/qdrant
```

### Start the Hive Server

```bash
./bin/hive-server
```

The server will:
- Start gRPC server on port 50051
- Start HTTP server on port 8080
- Connect to Qdrant on localhost:6334

### Start the Drone Client

```bash
mkdir -p watch
./bin/drone-client -watch-dir ./watch -hive-addr localhost:50051
```

Place PDF files in the `watch/` directory and they will be automatically processed and sent to the Hive.

## Docker Compose (Full Stack)

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

## Development Notes

- **Proto files**: After modifying `backend/proto/hive.proto`, run `make proto` to regenerate Go code
- **Database**: SQLite database is created at `./hive.db` by default (configurable via `-db-path`)
- **Logs**: Application logs go to stdout/stderr by default

## Next Steps (Implementation TODO)

The scaffolded code includes placeholders for:

1. **PDF Processing**: Implement actual PDF text extraction in `backend/internal/pdf/processor.go`
   - Consider using a library like `github.com/gen2brain/go-fitz` or `github.com/unidoc/unipdf`

2. **Vector Embeddings**: Add embedding generation
   - Consider using `github.com/tmc/langchaingo` for embeddings
   - Or call an external API like OpenAI embeddings

3. **Qdrant Integration**: Complete the vector database operations in `backend/internal/vectordb/vectordb.go`
   - Create collection on startup
   - Implement actual upsert/search/delete operations

4. **Web UI**: Enhance the templates in `frontend/template/`
   - Complete the search page
   - Add document upload UI
   - Show search results

5. **Error Handling**: Add comprehensive error handling and logging

6. **Configuration**: Move hardcoded values to config files

