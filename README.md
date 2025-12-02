# The Hive - Distributed RAG System

A production-ready RAG (Retrieval-Augmented Generation) system that allows distributed clients ("Drones") to upload documents (PDF, DOCX, XLSX, HTML, EML), which are indexed by a central server ("Hive") and made searchable via a web UI.

## Architecture

- **Hive Server**: Central Go server that handles document ingestion, indexing, and search
- **Drone Client**: Local Go client that watches for files (PDF, DOCX, XLSX, HTML, EML) and syncs them to the Hive
- **Qdrant**: Vector database for semantic search
- **SQLite**: Metadata storage
- **Embeddings**: Supports OpenAI, Ollama, or mock embedders
- **Web UI**: Go templates + HTMX + TailwindCSS

## Technology Stack

- **Language**: Go 1.23+
- **Communication**: gRPC with Protobuf
- **Vector DB**: Qdrant (Docker)
- **Metadata DB**: SQLite
- **Web UI**: Go `html/template` + HTMX
- **Styling**: TailwindCSS (CDN)
- **Deployment**: Docker Compose

## Project Structure

```
the-hive/
├── cmd/
│   ├── hive-server/    # Main Hive server binary
│   └── drone-client/   # Drone client binary
├── internal/
│   ├── proto/          # Generated protobuf code
│   ├── server/         # gRPC service implementation
│   ├── client/         # Drone client logic
│   ├── parser/         # Multimodal document parsers (PDF, DOCX, Excel, HTML, EML)
│   ├── embeddings/     # Embedding service (OpenAI, Ollama, mock)
│   ├── vectordb/       # Vector database abstraction
│   ├── queue/          # Job queue (Redis)
│   ├── worker/         # Background workers
│   └── jobs/           # Job handlers
├── proto/              # Protobuf definitions
├── frontend/
│   ├── template/       # Go HTML templates
│   └── static/         # Static assets
└── infra/
    ├── terraform/      # Infrastructure as Code
    ├── ansible/        # Configuration Management
    └── caddy/          # Reverse proxy configuration
```

## Getting Started

### Prerequisites

- Go 1.24+
- Docker and Docker Compose
- Protobuf compiler (`protoc`)
- MuPDF library (for PDF processing via go-fitz)
- Make (optional, but recommended)
- Redis (optional, for job queue)

**Quick Setup:**
```bash
# Run the setup script to install dependencies
./backend/SETUP.sh
```

### Build

1. Generate protobuf code:
```bash
make proto
```

2. Build binaries:
```bash
make build
```

Or build individually:
```bash
make build-hive
make build-drone
```

### Run with Docker Compose

```bash
docker-compose up -d
```

This will start:
- Qdrant on ports 6333 (REST) and 6334 (gRPC)
- Hive server on ports 8080 (HTTP) and 50051 (gRPC)
- Caddy reverse proxy on ports 80/443

### Run Locally

1. Start Qdrant:
```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

2. Start Hive server:
```bash
./bin/hive-server
```

3. Start Drone client:
```bash
./bin/drone-client -watch-dir ./watch
```

The drone client supports multiple file types:
- **PDF** files (`.pdf`)
- **Word documents** (`.docx`)
- **Excel spreadsheets** (`.xlsx`, `.xls`)
- **HTML files** (`.html`, `.htm`)
- **Email files** (`.eml`)

Place any supported file type in the watch directory and it will be automatically processed.

## Development

### Generate Protobuf Code

```bash
make proto
```

### Run Tests

```bash
make test
```

### Clean Build Artifacts

```bash
make clean
```

## Deployment

### Infrastructure (Terraform)

1. Set up DigitalOcean token:
```bash
export TF_VAR_do_token="your-token"
```

2. Initialize Terraform:
```bash
cd infra/terraform
terraform init
```

3. Plan and apply:
```bash
terraform plan
terraform apply
```

### Configuration (Ansible)

Update `infra/ansible/inventory.yml` with your droplet IP, then:

```bash
ansible-playbook -i inventory.yml playbook.yml
```

## Configuration

### Environment Variables

**Hive Server:**
- `EMBEDDER_TYPE`: Embedder type - `openai`, `ollama`, or `mock` (default: `mock`)
- `OPENAI_API_KEY`: OpenAI API key (required if using OpenAI embedder)
- `EMBEDDER_MODEL`: Model name (e.g., `text-embedding-3-small` for OpenAI)
- `OLLAMA_BASE_URL`: Ollama server URL (default: `http://localhost:11434`)
- `JOB_QUEUE_KEY`: Redis job queue key (default: `jobs:default`)

**Example:**
```bash
export EMBEDDER_TYPE=openai
export OPENAI_API_KEY=sk-...
export EMBEDDER_MODEL=text-embedding-3-small
./bin/hive-server
```

## API

### gRPC

- `Ingest(Chunk) -> Status`: Ingest a document chunk
- `Query(Search) -> Result`: Search for relevant documents

### HTTP

- `GET /`: Home page
- `GET /search`: Search page
- `POST /api/search`: Search API endpoint (accepts `query` parameter)
- `POST /api/jobs/recalc-priority`: Job queue endpoint

## License

MIT

