# Drone Client Upgrade - Complete

The drone-client has been upgraded into a robust, long-running service with web UI and configuration management.

## New Architecture

### Components Created

1. **Configuration Management** (`internal/drone/config.go`)
   - Uses `spf13/viper` for configuration
   - Auto-generates default config in `~/.the-hive/config.yaml`
   - Supports CLI flag overrides
   - Hot-reload capability

2. **Web Server** (`internal/drone/web/`)
   - Lightweight HTTP server (port 9090 by default)
   - Embedded HTML/JS dashboard using `//go:embed`
   - RESTful API endpoints
   - Server-Sent Events (SSE) for real-time updates

3. **Watcher Manager** (`internal/drone/watcher/manager.go`)
   - Recursive directory watching
   - Dynamic subdirectory detection
   - Multiple watch path support
   - Event broadcasting for UI updates

4. **Event System** (`internal/drone/web/events.go`)
   - SSE event broadcaster
   - Real-time file processing updates
   - Live log streaming

## Features

✅ **Configuration Management**
- Config file in `~/.the-hive/config.yaml`
- Auto-generated on first run
- CLI flags override config values
- Hot-reload on config save

✅ **Web Dashboard**
- Accessible at `http://localhost:9090`
- Add/remove watch paths
- Change server address
- Real-time status display
- Live event log

✅ **Recursive File Watching**
- Watches all subdirectories
- Automatically adds new directories
- Processes existing files on startup

✅ **Real-Time Updates**
- Server-Sent Events (SSE)
- Live log of file processing
- Status updates
- Error notifications

## Usage

### Build

```bash
make build-drone
```

### Run

```bash
# Use default config (~/.the-hive/config.yaml)
./bin/drone-client

# Override config file location
./bin/drone-client -config /path/to/config.yaml

# Override server address
./bin/drone-client -server localhost:50051

# Override watch directories (comma-separated)
./bin/drone-client -watch-dirs "./watch1,./watch2"

# Override web port
./bin/drone-client -web-port 9091
```

### Access Web UI

Open `http://localhost:9090` in your browser to:
- View current configuration
- Add/remove watch paths
- Change server address
- See real-time file processing logs
- Monitor status

## Configuration File Format

Default config at `~/.the-hive/config.yaml`:

```yaml
server:
  address: "localhost:50051"  # Hive server gRPC address

watch_paths:
  - "./watch"  # Directories to watch for files

web_server:
  port: 9090  # Web UI port
```

## API Endpoints

- `GET /api/config` - Get current configuration
- `POST /api/config/save` - Save configuration (hot-reloads watcher)
- `GET /api/status` - Get current status
- `GET /api/stream` - SSE event stream
- `GET /api/watch-paths` - Get watch paths
- `POST /api/watch-paths/add` - Add watch path
- `POST /api/watch-paths/remove` - Remove watch path

## Event Types

SSE events broadcast:
- `file_detected` - New file detected
- `file_processing` - File being processed
- `file_complete` - File successfully processed
- `file_error` - Error processing file

## Migration from Old Client

The old command-line flags still work:
- `-watch-dir` → `-watch-dirs` (now supports multiple)
- `-hive-addr` → `-server`
- New: `-config` for config file path
- New: `-web-port` for web server port

