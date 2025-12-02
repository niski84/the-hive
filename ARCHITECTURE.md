# The Hive - Architecture Overview

## System Components

### 1. Hive Server (`backend/cmd/hive-server/`)
- **Purpose**: Central server that receives and indexes documents
- **Interfaces**:
  - gRPC server (port 50051): Receives chunks from drones
  - HTTP server (port 8080): Serves web UI and REST API
- **Dependencies**:
  - SQLite: Metadata storage
  - Qdrant: Vector database for semantic search
- **Key Files**:
  - `main.go`: Server entry point with gRPC/HTTP setup
  - `backend/internal/server/hive_service.go`: gRPC service implementation

### 2. Drone Client (`backend/cmd/drone-client/`)
- **Purpose**: Local client that watches for PDFs and syncs to Hive
- **Features**:
  - File system watching (`fsnotify`)
  - PDF text extraction (placeholder)
  - Text chunking
  - gRPC communication with Hive
- **Key Files**:
  - `main.go`: Client entry point with file watching
  - `backend/internal/client/drone_client.go`: Hive communication client

### 3. Communication Protocol
- **Protocol**: gRPC with Protobuf
- **Definition**: `backend/proto/hive.proto`
- **Services**:
  - `Ingest(Chunk) -> Status`: Upload document chunks
  - `Query(Search) -> Result`: Search indexed documents
- **Generated Code**: `backend/internal/proto/` (run `make proto` to generate)

### 4. Vector Database
- **Technology**: Qdrant (Docker container)
- **Interface**: `backend/internal/vectordb/vectordb.go`
- **Operations**: Upsert, Search, Delete
- **Status**: Placeholder implementation (needs embedding integration)

### 5. PDF Processing
- **Location**: `backend/internal/pdf/processor.go`
- **Functions**:
  - Text extraction (placeholder)
  - Text chunking with overlap
- **Status**: Placeholder implementation

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
1. PDF placed in watched directory
2. Drone client detects file change
3. PDF text extracted and chunked
4. Chunks sent via gRPC to Hive
5. Hive stores:
   - Metadata in SQLite
   - Vectors in Qdrant (after embedding)
6. Status returned to Drone

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
├── backend/
│   ├── cmd/
│   │   ├── hive-server/      # Hive server binary
│   │   └── drone-client/     # Drone client binary
│   ├── internal/
│   │   ├── client/           # Drone client logic
│   │   ├── pdf/              # PDF processing
│   │   ├── proto/            # Generated protobuf code
│   │   ├── server/           # gRPC service implementation
│   │   └── vectordb/         # Vector database abstraction
│   └── proto/                # Protobuf definitions
├── frontend/
│   ├── static/               # CSS, JS, images
│   └── template/             # Go HTML templates
├── infra/
│   ├── ansible/              # Configuration management
│   ├── caddy/                # Reverse proxy config
│   └── terraform/            # Infrastructure as Code
├── data/                     # Persistent data (gitignored)
├── logs/                     # Application logs (gitignored)
├── docker-compose.yml        # Full stack deployment
├── Dockerfile.hive-server    # Hive server container
├── Makefile                  # Build automation
└── go.mod                    # Go dependencies
```

## Next Implementation Steps

1. **PDF Processing**: Integrate real PDF library
2. **Embeddings**: Add embedding model/API integration
3. **Qdrant**: Complete vector database operations
4. **Web UI**: Enhance search and results display
5. **Error Handling**: Add comprehensive error handling
6. **Testing**: Add unit and integration tests
7. **Configuration**: Externalize configuration
8. **Authentication**: Add security/auth if needed

