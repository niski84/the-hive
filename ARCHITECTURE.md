# The Hive - Architecture Overview

## System Components

### 1. Hive Server (`cmd/hive-server/`)
- **Purpose**: Central server that receives and indexes documents
- **Interfaces**:
  - gRPC server (port 50051): Receives chunks from drones
  - HTTP server (port 8080): Serves web UI and REST API
- **Dependencies**:
  - SQLite: Metadata storage
  - Qdrant: Vector database for semantic search
  - Redis: Job queue (optional)
  - Embedding service: OpenAI, Ollama, or mock
- **Key Files**:
  - `cmd/hive-server/main.go`: Server entry point with gRPC/HTTP setup
  - `internal/server/hive_service.go`: gRPC service implementation
  - `internal/embeddings/`: Embedding service implementations

### 2. Drone Client (`cmd/drone-client/`)
- **Purpose**: Local client that watches for files and syncs to Hive
- **Features**:
  - File system watching (`fsnotify`)
  - Multimodal document parsing (PDF, DOCX, Excel, HTML, EML)
  - Text chunking with overlap
  - gRPC communication with Hive
  - Temporary file filtering
- **Key Files**:
  - `cmd/drone-client/main.go`: Client entry point with file watching
  - `internal/client/drone_client.go`: Hive communication client
  - `internal/parser/`: Multimodal document parsers

### 3. Communication Protocol
- **Protocol**: gRPC with Protobuf
- **Definition**: `proto/hive.proto`
- **Services**:
  - `Ingest(Chunk) -> Status`: Upload document chunks
  - `Query(Search) -> Result`: Search indexed documents
- **Generated Code**: `internal/proto/` (run `make proto` to generate)

### 4. Vector Database
- **Technology**: Qdrant (Docker container)
- **Interface**: `internal/vectordb/vectordb.go`
- **Operations**: Upsert, Search, Delete
- **Status**: Implementation structure complete, API calls need verification

### 5. Document Parsing
- **Location**: `internal/parser/`
- **Supported Formats**:
  - PDF: `parser/pdf.go` (using go-fitz/MuPDF)
  - DOCX: `parser/docx.go` (using nguyenthenguyen/docx)
  - Excel: `parser/excel.go` (using xuri/excelize with markdownification)
  - HTML: `parser/html.go` (using PuerkitoBio/goquery, removes scripts/styles)
  - EML: `parser/email.go` (using mnako/letters)
- **Features**:
  - Strategy Pattern for file type routing
  - Text chunking with configurable overlap
  - Temporary file filtering

### 6. Web UI
- **Technology**: Go `html/template` + HTMX + TailwindCSS
- **Location**: `frontend/template/`
- **Pages**:
  - `base.html`: Base layout
  - `index.html`: Home page
  - `search.html`: Search interface
- **Assets**: `frontend/static/`

## Deployment Architecture

### Development (Local)
```
┌─────────────┐     ┌──────────────┐     ┌──────────┐
│ Drone Client│────▶│ Hive Server  │────▶│  Qdrant  │
│  (Local)    │     │  (Local)     │     │ (Docker) │
└─────────────┘     └──────────────┘     └──────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Web UI     │
                    │  (Port 8080) │
                    └──────────────┘
```

### Production (Docker Compose)
```
┌─────────────┐
│   Caddy     │  (Reverse Proxy + SSL)
│ (Port 80/443│
└──────┬──────┘
       │
       ▼
┌──────────────┐     ┌──────────┐
│ Hive Server  │────▶│  Qdrant  │
│  (Port 8080) │     │ (Port 6333/6334)
└──────────────┘     └──────────┘
       │
       ▼
  ┌────────┐
  │ SQLite │
  │  (File)│
  └────────┘
```

### Infrastructure (Terraform)
- **Provider**: DigitalOcean
- **Resources**: Droplet (Ubuntu 24.04, 2GB RAM)
- **Configuration**: Ansible playbook for server setup

## Data Flow

### Document Ingestion
1. File (PDF, DOCX, Excel, HTML, or EML) placed in watched directory
2. Drone client detects file change (skips temporary files)
3. File routed to appropriate parser based on extension
4. Text extracted and chunked with overlap
5. Chunks sent via gRPC to Hive
6. Hive processes:
   - Stores metadata in SQLite
   - Generates embeddings (if not provided)
   - Stores vectors in Qdrant
7. Status returned to Drone

### Search Flow
1. User submits query via Web UI
2. HTTP request to Hive server
3. Query embedded to vector
4. Vector search in Qdrant
5. Results retrieved with metadata from SQLite
6. Results displayed in Web UI

## File Structure

```
the-hive/
├── cmd/
│   ├── hive-server/          # Hive server binary
│   └── drone-client/         # Drone client binary
├── internal/
│   ├── client/               # Drone client logic
│   ├── parser/              # Multimodal document parsers
│   │   ├── pdf.go           # PDF parser (go-fitz)
│   │   ├── docx.go          # DOCX parser
│   │   ├── excel.go         # Excel parser
│   │   ├── html.go          # HTML parser
│   │   ├── email.go         # EML parser
│   │   ├── dispatcher.go    # File type router
│   │   └── chunker.go        # Text chunking
│   ├── embeddings/          # Embedding service
│   │   ├── embeddings.go    # Interface and factory
│   │   ├── openai.go        # OpenAI embedder
│   │   ├── ollama.go        # Ollama embedder
│   │   └── mock.go          # Mock embedder
│   ├── proto/               # Generated protobuf code
│   ├── server/              # gRPC service implementation
│   ├── vectordb/            # Vector database abstraction
│   ├── queue/               # Job queue (Redis)
│   ├── worker/              # Background workers
│   └── jobs/                # Job handlers
├── proto/                   # Protobuf definitions
├── frontend/
│   ├── static/              # CSS, JS, images
│   └── template/            # Go HTML templates
├── infra/
│   ├── ansible/             # Configuration management
│   ├── caddy/               # Reverse proxy config
│   └── terraform/           # Infrastructure as Code
├── backend/                 # Legacy directory (can be removed)
├── data/                    # Persistent data (gitignored)
├── logs/                    # Application logs (gitignored)
├── docker-compose.yml       # Full stack deployment
├── Dockerfile.hive-server   # Hive server container
├── Makefile                 # Build automation
└── go.mod                   # Go dependencies
```

## Implementation Status

✅ **Completed:**
- PDF text extraction (go-fitz/MuPDF)
- Multimodal document parsing (PDF, DOCX, Excel, HTML, EML)
- Embedding service with multiple backends
- Text chunking with overlap
- Search API endpoint
- Temporary file filtering
- Docker setup with CGO support

⚠️ **In Progress:**
- Qdrant API integration (structure complete, needs API verification)

📋 **Next Steps:**
1. **Qdrant Operations**: Verify and complete Qdrant client API calls
2. **Web UI**: Enhance search results display
3. **Document Management**: Add endpoints for viewing/deleting documents
4. **Error Handling**: Improve error handling and retries
5. **Testing**: Add unit and integration tests
6. **Monitoring**: Add metrics and observability
7. **Authentication**: Add security/auth if needed

