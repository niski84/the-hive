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
3. **MuPDF Library** (Required for PDF processing):
   ```bash
   # Ubuntu/Debian
   sudo apt-get install libmupdf-dev build-essential pkg-config
   
   # macOS
   brew install mupdf
   ```
4. **Protobuf Go Plugins**:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```
5. **Docker & Docker Compose**: For running Qdrant and deployment
6. **Redis** (Optional): For job queue functionality

## Quick Setup

Run the automated setup script:

```bash
./backend/SETUP.sh
```

This will install all dependencies and set up your environment.

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
   This will generate the gRPC code in `internal/proto/`

3. **Build the binaries**:
   ```bash
   make build
   ```
   This creates:
   - `bin/hive-server` - The Hive server binary
   - `bin/drone-client` - The Drone client binary

   **Note**: Building requires CGO to be enabled (for go-fitz and sqlite). The Makefile handles this automatically.

## Running Locally

### Start Qdrant (Vector Database)

```bash
docker run -d -p 6333:6333 -p 6334:6334 --name qdrant qdrant/qdrant
```

### Start the Hive Server

```bash
# With default settings (mock embedder)
./bin/hive-server

# With OpenAI embedder
export EMBEDDER_TYPE=openai
export OPENAI_API_KEY=sk-your-key-here
export EMBEDDER_MODEL=text-embedding-3-small
./bin/hive-server

# With Ollama embedder (local)
export EMBEDDER_TYPE=ollama
export OLLAMA_BASE_URL=http://localhost:11434
export EMBEDDER_MODEL=nomic-embed-text
./bin/hive-server
```

The server will:
- Start gRPC server on port 50051
- Start HTTP server on port 8080
- Connect to Qdrant on localhost:6334
- Initialize embedder based on `EMBEDDER_TYPE` environment variable

### Start the Drone Client

```bash
mkdir -p watch
./bin/drone-client -watch-dir ./watch -hive-addr localhost:50051
```

Place supported files in the `watch/` directory and they will be automatically processed and sent to the Hive.

**Supported file types:**
- PDF files (`.pdf`)
- Word documents (`.docx`)
- Excel spreadsheets (`.xlsx`, `.xls`)
- HTML files (`.html`, `.htm`)
- Email files (`.eml`)

The drone client automatically detects file types and routes them to the appropriate parser.

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

- **Proto files**: After modifying `proto/hive.proto`, run `make proto` to regenerate Go code
- **Database**: SQLite database is created at `./hive.db` by default (configurable via `-db-path`)
- **Logs**: Application logs go to stdout/stderr by default
- **CGO**: The project requires CGO for PDF processing (go-fitz) and SQLite. Ensure `CGO_ENABLED=1` when building.

## Environment Variables

### Hive Server

- `EMBEDDER_TYPE`: Embedder type (`openai`, `ollama`, or `mock`) - default: `mock`
- `OPENAI_API_KEY`: OpenAI API key (required for OpenAI embedder)
- `EMBEDDER_MODEL`: Model name (e.g., `text-embedding-3-small`)
- `OLLAMA_BASE_URL`: Ollama server URL - default: `http://localhost:11434`
- `JOB_QUEUE_KEY`: Redis job queue key - default: `jobs:default`
- `GRPC_PORT`: gRPC server port - default: `50051`
- `HTTP_PORT`: HTTP server port - default: `8080`
- `DB_PATH`: SQLite database path - default: `./hive.db`

### Drone Client

- `WATCH_DIR`: Directory to watch for files - default: `./watch`
- `HIVE_ADDR`: Hive server address - default: `localhost:50051`

## Implementation Status

✅ **Completed:**
- PDF text extraction using go-fitz
- Multimodal document parsing (PDF, DOCX, Excel, HTML, EML)
- Embedding service with multiple backends (OpenAI, Ollama, mock)
- Search API endpoint with real embeddings
- Text chunking with overlap
- Temporary file filtering

⚠️ **In Progress / TODO:**
- Qdrant API integration (needs verification of client API structure)
- Enhanced search results UI
- Document management endpoints
- Error handling improvements
- Monitoring and observability

