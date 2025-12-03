# Quick Start - UI-Only Mode

You can run the Hive server in **UI-only mode** to view the web interface without needing Qdrant or other services running.

## Build the Server

```bash
# Generate protobuf code (if not already done)
make proto

# Build the Hive server
make build-hive
```

This creates `bin/hive-server`.

## Run in UI-Only Mode

Simply run the server without Qdrant:

```bash
./bin/hive-server
```

The server will:
- ✅ Start on `http://localhost:8081` (default port changed to avoid conflicts)
- ✅ Serve the web UI (home page, search page)
- ✅ Use a mock vector database (no Qdrant needed)
- ✅ Use a mock embedder (no API keys needed)
- ⚠️ Search will return empty results (but UI works!)

## What You'll See

1. **Home Page** (`http://localhost:8081/`):
   - Welcome page with project overview
   - Link to search page

2. **Search Page** (`http://localhost:8081/search`):
   - Search input form
   - Results area (will show "no results" in UI-only mode)

## Full Functionality

To enable full search functionality:

1. **Start Qdrant**:
   ```bash
   docker run -d -p 6333:6333 -p 6334:6334 --name qdrant qdrant/qdrant
   ```

2. **Restart the server** - it will automatically connect to Qdrant

3. **Add documents** using the drone client:
   ```bash
   make build-drone
   mkdir -p watch
   ./bin/drone-client -watch-dir ./watch
   ```

4. **Search** - now returns real results!

## Environment Variables (Optional)

- `EMBEDDER_TYPE`: `mock` (default), `openai`, or `ollama`
- `HTTP_PORT`: Server port (default: 8081)
- `GRPC_PORT`: gRPC port (default: 50051)
- `DB_PATH`: SQLite database path (default: `./hive.db`)

## Troubleshooting

- **Port already in use**: Change port with `-http-port 8082` (or any available port)
- **Can't find templates**: Use `-template-dir ./frontend/template`
- **Build errors**: Make sure CGO is enabled and MuPDF libraries are installed

