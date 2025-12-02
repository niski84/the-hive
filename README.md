# The Hive - Distributed RAG System

A production-ready RAG (Retrieval-Augmented Generation) system that allows distributed clients ("Drones") to upload PDFs, which are indexed by a central server ("Hive") and made searchable via a web UI.

## Architecture

- **Hive Server**: Central Go server that handles document ingestion, indexing, and search
- **Drone Client**: Local Go client that watches for PDF files and syncs them to the Hive
- **Qdrant**: Vector database for semantic search
- **SQLite**: Metadata storage
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
├── infra/
│   ├── terraform/      # Infrastructure as Code
│   ├── ansible/        # Configuration Management
│   └── caddy/          # Reverse proxy configuration
├── backend/
│   ├── cmd/
│   │   ├── hive-server/    # Main Hive server binary
│   │   └── drone-client/   # Drone client binary
│   ├── internal/
│   │   ├── proto/      # Generated protobuf code
│   │   ├── server/     # gRPC service implementation
│   │   ├── client/     # Drone client logic
│   │   ├── pdf/        # PDF processing
│   │   └── vectordb/   # Vector database abstraction
│   └── proto/          # Protobuf definitions
└── frontend/
    ├── template/       # Go HTML templates
    └── static/         # Static assets
```

## Getting Started

### Prerequisites

- Go 1.24+
- Docker and Docker Compose
- Protobuf compiler (`protoc`)
- Make (optional, but recommended)

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

## API

### gRPC

- `Ingest(Chunk) -> Status`: Ingest a document chunk
- `Query(Search) -> Result`: Search for relevant documents

### HTTP

- `GET /`: Home page
- `GET /search`: Search page
- `POST /api/search`: Search API endpoint

## License

MIT

