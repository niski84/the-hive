// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/the-hive/internal/config"
	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/jobs"
	"github.com/the-hive/internal/logger"
	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/queue"
	"github.com/the-hive/internal/server"
	"github.com/the-hive/internal/vectordb"
	"github.com/the-hive/internal/worker"
)

var (
	grpcPort    = flag.Int("grpc-port", 50051, "gRPC server port")
	httpPort    = flag.Int("http-port", 8081, "HTTP server port")
	dbPath      = flag.String("db-path", "./hive.db", "SQLite database path")
	templateDir = flag.String("template-dir", "./frontend/template", "Template directory")
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

	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		logger.Printf("No .env file found, using environment variables: %v", err)
	} else {
		logger.Printf("Loaded .env file")
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

	if err := initDatabase(db); err != nil {
		logger.Fatalf("failed to initialize schema: %v", err)
	}

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

	grpcServer := grpc.NewServer()
	hiveService := server.NewHiveService(db, vectorDB, embedder)
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
		Handler: routes(db, vectorDB, embedder, jobQueue, *templateDir, *staticDir),
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

func routes(db *sql.DB, vectorDB vectordb.VectorDB, embedder embeddings.Embedder, jobQueue queue.Queue, templateDir, staticDir string) http.Handler {
	_ = db
	_ = vectorDB
	mux := http.NewServeMux()

	staticPath, _ := filepath.Abs(staticDir)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	// Helper function to render templates
	renderTemplate := func(w http.ResponseWriter, tmplName string, data interface{}) {
		basePath := filepath.Join(templateDir, "base.html")
		tmplPath := filepath.Join(templateDir, tmplName)

		tmpl, err := template.ParseFiles(basePath, tmplPath)
		if err != nil {
			log.Printf("failed to parse template %s: %v", tmplName, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
			log.Printf("failed to execute template %s: %v", tmplName, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Create handlers with dependencies
	ingestHandler := server.NewIngestHandler(vectorDB)
	searchHandler := server.NewSearchHandler(vectorDB, embedder)

	// Web interface handlers
	mux.HandleFunc("/", server.HandleWeb)
	mux.HandleFunc("/settings", server.HandleSettings)

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "search.html", nil)
	})

	// API endpoints
	mux.HandleFunc("/api/v1/ingest", ingestHandler.HandleIngest)
	mux.HandleFunc("/api/v1/search", searchHandler.HandleSearch)

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

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":"method not allowed"}`))
			return
		}

		query := r.FormValue("query")
		if query == "" {
			// Try JSON body
			var req struct {
				Query string `json:"query"`
				TopK  int    `json:"top_k"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				query = req.Query
			}
		}

		if query == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"query parameter is required"}`))
			return
		}

		topK := 10
		if topKStr := r.FormValue("top_k"); topKStr != "" {
			fmt.Sscanf(topKStr, "%d", &topK)
		}

		ctx := r.Context()

		// Generate query embedding
		queryVector, err := embedder.EmbedText(ctx, query)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<div class="bg-yellow-50 p-4 rounded-lg text-yellow-700">Search is not available: failed to generate embedding. Please ensure Qdrant is running for full functionality.</div>`)
			return
		}

		// Search in vector database
		matches, err := vectorDB.Search(ctx, queryVector, topK)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<div class="bg-yellow-50 p-4 rounded-lg text-yellow-700">Search is not available: %v. Please ensure Qdrant is running for full functionality.</div>`, err)
			return
		}

		// Fetch content from SQLite for each match
		type searchMatch struct {
			ChunkID    string
			DocumentID string
			Content    string
			Score      float32
			Metadata   map[string]string
		}

		results := make([]searchMatch, 0, len(matches))
		for _, match := range matches {
			var content string
			if err := db.QueryRowContext(ctx, "SELECT content FROM chunks WHERE id = ?", match.ID).Scan(&content); err != nil {
				// Skip matches without content
				continue
			}

			results = append(results, searchMatch{
				ChunkID:    match.ID,
				DocumentID: match.DocumentID,
				Content:    content,
				Score:      match.Score,
				Metadata:   match.Metadata,
			})
		}

		// Render HTML template for HTMX
		tmplPath := filepath.Join(templateDir, "search_results.html")
		tmpl, err := template.ParseFiles(tmplPath)
		if err != nil {
			log.Printf("failed to parse search results template: %v", err)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `<div class="bg-red-50 p-4 rounded-lg text-red-700">Error rendering results: %v</div>`, err)
			return
		}

		data := map[string]interface{}{
			"Matches": results,
			"Count":   len(results),
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("failed to execute search results template: %v", err)
			fmt.Fprintf(w, `<div class="bg-red-50 p-4 rounded-lg text-red-700">Error rendering results: %v</div>`, err)
		}
	})

	// Job queue API endpoint
	mux.HandleFunc("/api/jobs/recalc-priority", func(w http.ResponseWriter, r *http.Request) {
		if jobQueue == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"job queue not available"}`))
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":"method not allowed"}`))
			return
		}

		// Parse request body
		var payload jobs.RecalcIssuePriorityPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf(`{"error":"invalid request: %v"}`, err)))
			return
		}

		// Enqueue job
		ctx := r.Context()
		if err := jobs.EnqueueRecalcIssuePriority(ctx, jobQueue, payload); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error":"failed to enqueue job: %v"}`, err)))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"job enqueued"}`))
	})

	return mux
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
