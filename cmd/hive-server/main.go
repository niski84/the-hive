package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/the-hive/internal/config"
	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/jobs"
	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/queue"
	"github.com/the-hive/internal/server"
	"github.com/the-hive/internal/vectordb"
	"github.com/the-hive/internal/worker"
)

var (
	grpcPort    = flag.Int("grpc-port", 50051, "gRPC server port")
	httpPort    = flag.Int("http-port", 8080, "HTTP server port")
	dbPath      = flag.String("db-path", "./hive.db", "SQLite database path")
	templateDir = flag.String("template-dir", "./frontend/template", "Template directory")
	staticDir   = flag.String("static-dir", "./frontend/static", "Static assets directory")
	workerCount = flag.Int("worker-count", 5, "Number of background workers")
)

func main() {
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("failed to open sqlite database: %v", err)
	}
	defer db.Close()

	if err := initDatabase(db); err != nil {
		log.Fatalf("failed to initialize schema: %v", err)
	}

	// Connect to Qdrant via gRPC
	qdrantConn, err := grpc.Dial("localhost:6334", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to Qdrant: %v", err)
	}
	defer qdrantConn.Close()

	qdrantClient := qdrant.NewQdrantClient(qdrantConn)

	vectorDB, err := vectordb.NewQdrantVectorDB(qdrantClient)
	if err != nil {
		log.Fatalf("failed to init vector db: %v", err)
	}

	// Initialize embedder
	embedderType := os.Getenv("EMBEDDER_TYPE")
	if embedderType == "" {
		embedderType = "mock" // default to mock for development
	}
	embedderConfig := map[string]string{
		"api_key":   os.Getenv("OPENAI_API_KEY"),
		"model":     os.Getenv("EMBEDDER_MODEL"),
		"base_url":  os.Getenv("OLLAMA_BASE_URL"),
		"dimension": os.Getenv("EMBEDDER_DIMENSION"),
	}
	embedder, err := embeddings.NewEmbedder(embedderType, embedderConfig)
	if err != nil {
		log.Fatalf("failed to initialize embedder: %v", err)
	}
	log.Printf("Initialized embedder: %s (dimension: %d)", embedderType, embedder.Dimension())

	// Initialize Redis and job queue
	ctx := context.Background()
	redisClient, err := config.NewRedisClient(ctx)
	if err != nil {
		log.Printf("warning: failed to connect to Redis: %v, job queue will not be available", err)
		redisClient = nil
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
			log.Fatalf("failed to create job queue: %v", err)
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
				log.Printf("unknown job type: %s", job.Type)
				return nil
			}
		}

		go func() {
			log.Printf("Starting %d background workers", *workerCount)
			if err := worker.StartWorkers(workerCtx, jobQueue, handler, *workerCount); err != nil {
				log.Printf("worker error: %v", err)
			}
		}()
	}

	grpcServer := grpc.NewServer()
	hiveService := server.NewHiveService(db, vectorDB, embedder)
	proto.RegisterHiveServer(grpcServer, hiveService)

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", *grpcPort))
	if err != nil {
		log.Fatalf("failed to listen on grpc port: %v", err)
	}

	go func() {
		log.Printf("gRPC server listening on %d", *grpcPort)
		if err := grpcServer.Serve(grpcListener); err != nil && err != grpc.ErrServerStopped {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: routes(db, vectorDB, embedder, jobQueue, *templateDir, *staticDir),
	}

	go func() {
		log.Printf("HTTP server listening on %d", *httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	waitForShutdown(grpcServer, httpServer, workerCancel)
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

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(templateDir, "index.html"))
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(templateDir, "search.html"))
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error":"failed to generate embedding: %v"}`, err)))
			return
		}

		// Search in vector database
		matches, err := vectorDB.Search(ctx, queryVector, topK)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error":"search failed: %v"}`, err)))
			return
		}

		// Fetch content from SQLite for each match
		type searchMatch struct {
			ChunkID    string            `json:"chunk_id"`
			DocumentID string            `json:"document_id"`
			Content    string            `json:"content"`
			Score      float32           `json:"score"`
			Metadata   map[string]string `json:"metadata"`
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

		response := map[string]interface{}{
			"matches": results,
			"count":   len(results),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
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

	log.Println("Shutting down servers...")

	// Stop workers
	if workerCancel != nil {
		workerCancel()
	}

	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
}
