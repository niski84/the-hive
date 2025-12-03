// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/the-hive/internal/config"
	"github.com/the-hive/internal/database"
	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/jobs"
	"github.com/the-hive/internal/logger"
	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/queue"
	"github.com/the-hive/internal/rules"
	"github.com/the-hive/internal/server"
	"github.com/the-hive/internal/server/middleware"
	"github.com/the-hive/internal/vectordb"
	"github.com/the-hive/internal/worker"
)

var (
	grpcPort    = flag.Int("grpc-port", 50051, "gRPC server port")
	httpPort    = flag.Int("http-port", 8081, "HTTP server port")
	dbPath      = flag.String("db-path", "./hive.db", "SQLite database path")
	templateDir = flag.String("template-dir", "./internal/server/templates", "Template directory")
	staticDir   = flag.String("static-dir", "./frontend/static", "Static assets directory")
	workerCount = flag.Int("worker-count", 5, "Number of background workers")
)

func main() {
	// Initialize logger first (before loading .env so we can log the process)
	logFile := "hive-server.log"
	if _, err := logger.Init(logFile); err != nil {
		log.Printf("Failed to initialize logger: %v, using stdout only", err)
	} else {
		logger.Printf("Logger initialized, writing to %s", logFile)
	}

	// DEBUG: Check if OPENAI_API_KEY exists BEFORE loading .env
	apiKeyBeforeLoad := os.Getenv("OPENAI_API_KEY")
	logger.Printf("[DEBUG] OPENAI_API_KEY before .env load: present=%v, length=%d", apiKeyBeforeLoad != "", len(apiKeyBeforeLoad))
	if apiKeyBeforeLoad != "" {
		preview := apiKeyBeforeLoad
		if len(preview) > 5 {
			preview = preview[:5]
		}
		logger.Printf("[DEBUG] Key Value (First 5 chars) before load: %s", preview)
	}

	// Load .env file if it exists (ignore error if file doesn't exist)
	envFileExists := false
	if _, err := os.Stat(".env"); err == nil {
		envFileExists = true
	}
	logger.Printf("[DEBUG] .env file found: %v", envFileExists)

	if err := godotenv.Load(); err != nil {
		logger.Printf("No .env file found, using environment variables: %v", err)
	} else {
		logger.Printf("Loaded .env file")
	}

	// DEBUG: Check API key source after loading .env
	apiKeyAfterLoad := os.Getenv("OPENAI_API_KEY")
	logger.Printf("[DEBUG] OPENAI_API_KEY present in environment: %v", apiKeyAfterLoad != "")
	logger.Printf("[DEBUG] OPENAI_API_KEY length: %d", len(apiKeyAfterLoad))
	
	if apiKeyAfterLoad != "" {
		preview := apiKeyAfterLoad
		if len(preview) > 5 {
			preview = preview[:5]
		}
		logger.Printf("[DEBUG] Key Value (First 5 chars): %s", preview)
		
		// Determine source
		if apiKeyBeforeLoad == "" && apiKeyAfterLoad != "" {
			logger.Printf("[DEBUG] Key source: .env file (was not in environment before)")
		} else if apiKeyBeforeLoad != "" && apiKeyBeforeLoad == apiKeyAfterLoad {
			logger.Printf("[DEBUG] Key source: Environment variable (unchanged by .env)")
		} else if apiKeyBeforeLoad != "" && apiKeyBeforeLoad != apiKeyAfterLoad {
			logger.Printf("[DEBUG] Key source: .env file (overrode environment variable)")
		}
	} else {
		logger.Printf("[DEBUG] Key source: Not found (neither in environment nor .env)")
	}

	// Check and set INSTALL_DATE if missing
	installDate := os.Getenv("INSTALL_DATE")
	if installDate == "" {
		// Set install date to today
		today := time.Now().Format("2006-01-02")
		os.Setenv("INSTALL_DATE", today)
		// Write to .env file if it exists
		if err := writeEnvVar("INSTALL_DATE", today); err != nil {
			logger.Printf("Warning: Failed to write INSTALL_DATE to .env: %v", err)
		} else {
			logger.Printf("Set INSTALL_DATE to %s", today)
		}
	} else {
		logger.Printf("INSTALL_DATE is set to: %s", installDate)
	}

	// Verify environment variables are loaded
	apiKeyLen := len(os.Getenv("OPENAI_API_KEY"))
	logger.Printf("Loaded API Key length: %d", apiKeyLen)
	if apiKeyLen > 0 {
		logger.Printf("OPENAI_API_KEY is set (length: %d)", apiKeyLen)
	} else {
		logger.Printf("OPENAI_API_KEY is not set - will use dummy embeddings")
	}

	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		logger.Fatalf("failed to open sqlite database: %v", err)
	}
	defer db.Close()

	// Enable WAL mode for concurrent read/write access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		logger.Fatalf("failed to enable WAL mode: %v", err)
	}
	logger.Printf("SQLite WAL mode enabled")

	// Set busy timeout to prevent indefinite hanging (10 seconds)
	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		logger.Fatalf("failed to set busy timeout: %v", err)
	}
	logger.Printf("SQLite busy timeout set to 10000ms")
	logger.Printf("SQLite busy timeout set to 5000ms")

	if err := initDatabase(db); err != nil {
		logger.Fatalf("failed to initialize schema: %v", err)
	}

	// Initialize event logger
	eventLogger, err := database.NewEventLogger(db)
	if err != nil {
		logger.Fatalf("failed to initialize event logger: %v", err)
	}

	// Initialize graph store
	graphStore, err := database.NewGraphStore(db)
	if err != nil {
		logger.Fatalf("failed to initialize graph store: %v", err)
	}

	// Initialize API key store
	apiKeyStore, err := database.NewAPIKeyStore(db)
	if err != nil {
		logger.Fatalf("failed to initialize API key store: %v", err)
	}

	// Initialize audit log store
	auditLogStore, err := database.NewAuditLogStore(db)
	if err != nil {
		logger.Fatalf("failed to initialize audit log store: %v", err)
	}

	// Initialize rules store
	ruleStore, err := rules.NewStore(db)
	if err != nil {
		logger.Fatalf("failed to initialize rules store: %v", err)
	}

	// Initialize system metadata store (for install_date, license_key, etc.)
	metadataStore, err := database.NewSystemMetadataStore(db)
	if err != nil {
		logger.Fatalf("failed to initialize system metadata store: %v", err)
	}
	
	// Ensure install_date is set in the database
	if err := metadataStore.EnsureInstallDate(); err != nil {
		logger.Fatalf("failed to ensure install_date: %v", err)
	}
	logger.Printf("System metadata initialized")

	// Connect to Qdrant via gRPC (optional - will use mock if unavailable)
	var vectorDB vectordb.VectorDB
	qdrantConn, err := grpc.Dial("localhost:6334", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("warning: failed to connect to Qdrant: %v, using mock vector DB", err)
		log.Printf("UI-only mode: Search functionality will be disabled")
		vectorDB = vectordb.NewMockVectorDB()
	} else {
		defer qdrantConn.Close()
		// Create Qdrant client (kept for compatibility, but vectordb uses connection directly)
		_ = qdrant.NewQdrantClient(qdrantConn)

		var vdbErr error
		vectorDB, vdbErr = vectordb.NewQdrantVectorDB(qdrantConn)
		if vdbErr != nil {
			log.Printf("warning: failed to init vector db: %v, using mock vector DB", vdbErr)
			log.Printf("UI-only mode: Search functionality will be disabled")
			vectorDB = vectordb.NewMockVectorDB()
		} else {
			log.Printf("Connected to Qdrant successfully")
		}
	}

	// Initialize embedder (after .env is loaded)
	embedder := initEmbedder()

	// Initialize Redis and job queue
	ctx := context.Background()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisClient, err := config.NewRedisClient(ctx)
	if err != nil {
		logger.Warnf("failed to connect to Redis at %s: %v, job queue will not be available", redisURL, err)
		redisClient = nil
	} else {
		logger.Printf("Connected to Redis at %s", redisURL)
	}

	var jobQueue queue.Queue
	var workerCancel context.CancelFunc
	if redisClient != nil {
		queueKey := os.Getenv("JOB_QUEUE_KEY")
		if queueKey == "" {
			queueKey = "jobs:default"
		}
		jobQueue, err = queue.NewRedisQueue(redisClient, queueKey)
		if err != nil {
			logger.Fatalf("failed to create job queue: %v", err)
		}

		// Start background workers
		workerCtx, cancel := context.WithCancel(ctx)
		workerCancel = cancel

		// Create a handler that routes jobs to appropriate handlers
		handler := func(ctx context.Context, job queue.Job) error {
			switch job.Type {
			case jobs.JobTypeRecalcIssuePriority:
				return jobs.HandleRecalcIssuePriority(ctx, job)
			default:
				logger.Printf("unknown job type: %s", job.Type)
				return nil
			}
		}

		go func() {
			logger.Printf("Starting %d background workers", *workerCount)
			if err := worker.StartWorkers(workerCtx, jobQueue, handler, *workerCount); err != nil {
				logger.Errorf("worker error: %v", err)
			}
		}()
	}

	// Initialize WebSocket manager (before hiveService so we can pass it)
	wsManager := server.NewWebSocketManager(redisClient)

	// Initialize analyst worker pool
	notificationAdapterImpl := &notificationAdapter{wm: wsManager}
	analystPool := worker.NewAnalystPool(ruleStore, notificationAdapterImpl, graphStore, vectorDB, embedder, 3)
	analystPool.Start()
	defer analystPool.Stop()

	// Initialize tagging worker pool
	taggerPool := worker.NewTaggerPool(2) // 2 workers for tagging
	taggerPool.Start()
	defer taggerPool.Stop()

	grpcServer := grpc.NewServer()
	hiveService := server.NewHiveService(db, vectorDB, embedder)
	hiveService.SetWebSocketManager(wsManager)
	proto.RegisterHiveServer(grpcServer, hiveService)

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", *grpcPort))
	if err != nil {
		logger.Fatalf("failed to listen on grpc port: %v", err)
	}

	go func() {
		logger.Printf("gRPC server listening on %d", *grpcPort)
		if err := grpcServer.Serve(grpcListener); err != nil && err != grpc.ErrServerStopped {
			logger.Fatalf("gRPC server error: %v", err)
		}
	}()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: routes(db, vectorDB, embedder, jobQueue, wsManager, analystPool, taggerPool, ruleStore, eventLogger, graphStore, apiKeyStore, auditLogStore, metadataStore, *templateDir, *staticDir),
	}

	go func() {
		logger.Printf("HTTP server listening on %d", *httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("HTTP server error: %v", err)
		}
	}()

	waitForShutdown(grpcServer, httpServer, workerCancel)
}

// initEmbedder initializes the embedder after .env is loaded
func initEmbedder() embeddings.Embedder {
	embedderType := os.Getenv("EMBEDDER_TYPE")
	if embedderType == "" {
		// Auto-detect based on OPENAI_API_KEY
		if len(os.Getenv("OPENAI_API_KEY")) > 0 {
			embedderType = "openai"
			log.Printf("EMBEDDER_TYPE not set, auto-detected: openai (OPENAI_API_KEY found)")
		} else {
			embedderType = "mock" // default to mock for development
			log.Printf("EMBEDDER_TYPE not set, using: mock (no OPENAI_API_KEY)")
		}
	}

	embedderConfig := map[string]string{
		"api_key":   os.Getenv("OPENAI_API_KEY"),
		"model":     os.Getenv("EMBEDDER_MODEL"),
		"base_url":  os.Getenv("OLLAMA_BASE_URL"),
		"dimension": os.Getenv("EMBEDDER_DIMENSION"),
	}

	embedder, err := embeddings.NewEmbedder(embedderType, embedderConfig)
	if err != nil {
		logger.Fatalf("failed to initialize embedder: %v", err)
	}
	logger.Printf("Initialized embedder: %s (dimension: %d)", embedderType, embedder.Dimension())
	return embedder
}

// writeEnvVar writes a key-value pair to the .env file
func writeEnvVar(key, value string) error {
	envPath := ".env"

	// Read existing .env file
	content, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if key already exists
	lines := strings.Split(string(content), "\n")
	keyFound := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = key + "=" + value
			keyFound = true
			break
		}
	}

	// Append if not found
	if !keyFound {
		lines = append(lines, key+"="+value)
	}

	// Write back to file
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0644)
}

func initDatabase(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS documents (
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		metadata TEXT
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id TEXT PRIMARY KEY,
		document_id TEXT NOT NULL,
		content TEXT NOT NULL,
		chunk_index INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (document_id) REFERENCES documents(id)
	);

	CREATE INDEX IF NOT EXISTS idx_chunks_document_id ON chunks(document_id);
	`
	_, err := db.Exec(schema)
	return err
}

func routes(db *sql.DB, vectorDB vectordb.VectorDB, embedder embeddings.Embedder, jobQueue queue.Queue, wsManager *server.WebSocketManager, analystPool *worker.AnalystPool, taggerPool *worker.TaggerPool, ruleStore *rules.Store, eventLogger *database.EventLogger, graphStore *database.GraphStore, apiKeyStore *database.APIKeyStore, auditLogStore *database.AuditLogStore, metadataStore *database.SystemMetadataStore, templateDir, staticDir string) http.Handler {
	_ = db
	_ = vectorDB
	mux := http.NewServeMux()
	
	// Apply traffic logger middleware to all routes
	trafficLogger := middleware.TrafficLogger

	staticPath, _ := filepath.Abs(staticDir)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	// Create middleware
	authMiddleware := server.AuthMiddleware(apiKeyStore)
	// Use new licensing middleware from middleware package
	licensingMiddleware := middleware.LicenseMiddleware(metadataStore)

	// Create handlers with dependencies
	ingestHandler := server.NewIngestHandler(vectorDB, wsManager, analystPool, taggerPool, eventLogger, auditLogStore)
	searchHandler := server.NewSearchHandler(vectorDB, embedder, auditLogStore)

	// Web interface handlers (public)
	mux.HandleFunc("/", server.HandleWeb)
	mux.HandleFunc("/settings", server.HandleSettings)
	mux.HandleFunc("/timeline", server.HandleTimelinePage)
	mux.HandleFunc("/graph", server.HandleGraphPage)

	// Protected API endpoints
	// Ingest requires client API key authentication
	mux.Handle("/api/v1/ingest", licensingMiddleware(authMiddleware(http.HandlerFunc(ingestHandler.HandleIngest))))
	// Search requires licensing check only (no API key for web UI)
	mux.Handle("/api/v1/search", licensingMiddleware(http.HandlerFunc(searchHandler.HandleSearch)))

	// Configuration endpoints
	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			server.HandleGetConfig(w, r)
		} else if r.Method == http.MethodPost {
			server.HandleSaveConfig(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/logs/stream", server.HandleLogStream)

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		server.HandleStats(w, r, vectorDB, db)
	})

	// WebSocket endpoint (protected - auth happens in HandleWebSocket)
	// Note: WebSocket auth is handled via query parameter or header
	mux.Handle("/api/v1/ws", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsManager.HandleWebSocket(w, r)
	})))

	// Rules API endpoints
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			server.HandleGetRules(w, r, ruleStore)
		} else if r.Method == http.MethodPost {
			server.HandleAddRule(w, r, ruleStore)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/rules/add", func(w http.ResponseWriter, r *http.Request) {
		server.HandleAddRule(w, r, ruleStore)
	})
	mux.HandleFunc("/api/v1/rules/update", func(w http.ResponseWriter, r *http.Request) {
		server.HandleUpdateRule(w, r, ruleStore)
	})
	mux.HandleFunc("/api/v1/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		server.HandleDeleteRule(w, r, ruleStore)
	})

	// Health endpoint (public - no auth required, but tracks API keys if provided)
	server.SetHealthAPIKeyStore(apiKeyStore)
	mux.HandleFunc("/api/v1/health", server.HandleHealth)

	// API Key management endpoints (admin only - no auth required for now, can add admin auth later)
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		server.HandleListAPIKeys(w, r, apiKeyStore)
	})
	mux.HandleFunc("/api/v1/keys/generate", func(w http.ResponseWriter, r *http.Request) {
		server.HandleGenerateAPIKey(w, r, apiKeyStore)
	})
	mux.HandleFunc("/api/v1/keys/revoke", func(w http.ResponseWriter, r *http.Request) {
		server.HandleRevokeAPIKey(w, r, apiKeyStore)
	})

	// Timeline API endpoint
	mux.HandleFunc("/api/v1/timeline", func(w http.ResponseWriter, r *http.Request) {
		server.HandleTimeline(w, r, eventLogger)
	})

	// Graph API endpoint (returns JSON data)
	mux.HandleFunc("/api/v1/graph", func(w http.ResponseWriter, r *http.Request) {
		server.HandleGraph(w, r, graphStore)
	})

	// Audit log API endpoint
	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		server.HandleAuditLogs(w, r, auditLogStore)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		searchHandler.HandleSearch(w, r)
	})

	// Wrap all routes with traffic logger
	return trafficLogger(mux)
}

func waitForShutdown(grpcServer *grpc.Server, httpServer *http.Server, workerCancel context.CancelFunc) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Println("Shutting down servers...")

	// Stop workers
	if workerCancel != nil {
		workerCancel()
	}

	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Errorf("HTTP shutdown error: %v", err)
	}

	// Close logger
	if err := logger.GetDefault().Close(); err != nil {
		log.Printf("Failed to close logger: %v", err)
	}
}
