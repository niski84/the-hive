// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/the-hive/internal/client"
	"github.com/the-hive/internal/drone/database"
	"github.com/the-hive/internal/drone/events"
	"github.com/the-hive/internal/parser"
	"github.com/the-hive/internal/proto"
)

// Manager manages file watchers for multiple directories
type Manager struct {
	watchPaths       []string
	serverAddr       string
	clientID         string
	eventBroadcaster *events.Broadcaster
	watchers         map[string]*fsnotify.Watcher
	droneClient      *client.DroneClient
	chunker          *parser.Chunker
	debouncer        *Debouncer
	decisionEngine   *DecisionEngine
	clientDB         *database.ClientDB
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// Status represents the current watcher status
type Status struct {
	WatchingPaths []string `json:"watching_paths"`
	TotalFiles    int      `json:"total_files"`
	Processed     int      `json:"processed"`
	Errors        int      `json:"errors"`
}

// NewManager creates a new watcher manager
func NewManager(watchPaths []string, serverAddr string, grpcServerAddr string, clientID string, broadcaster *events.Broadcaster, configDir string) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize database
	clientDB, err := database.NewClientDB(configDir)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize client database: %w", err)
	}

	// Initialize decision engine
	decisionEngine := NewDecisionEngine(clientDB)

	// Initialize debouncer with 500ms delay (callback will be set in Start())
	debouncer := NewDebouncer(500*time.Millisecond, nil)

	mgr := &Manager{
		watchPaths:       watchPaths,
		serverAddr:       serverAddr,
		clientID:         clientID,
		eventBroadcaster: broadcaster,
		watchers:         make(map[string]*fsnotify.Watcher),
		chunker:          parser.NewChunker(),
		debouncer:        debouncer,
		decisionEngine:   decisionEngine,
		clientDB:         clientDB,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Only connect to Hive server if gRPC address is provided
	if grpcServerAddr != "" {
		// Sanitize gRPC address: remove http:// or https:// if present
		grpcAddr := strings.TrimPrefix(grpcServerAddr, "http://")
		grpcAddr = strings.TrimPrefix(grpcAddr, "https://")
		grpcAddr = strings.TrimSpace(grpcAddr)

		log.Printf("Connecting to Hive server gRPC endpoint: %s", grpcAddr)
		conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			cancel()
			// Don't close clientDB here - let main() handle it via watcherMgr.Stop()
			return nil, fmt.Errorf("failed to connect to Hive server gRPC: %w", err)
		}

		hiveClient := proto.NewHiveClient(conn)
		mgr.droneClient = client.NewDroneClient(hiveClient)
		log.Printf("Successfully connected to Hive server gRPC endpoint")
	} else {
		log.Printf("Warning: No Hive server gRPC address configured. File watching will work but ingestion is disabled.")
	}

	return mgr, nil
}

// Start starts watching all configured paths
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set up debouncer callback to process files after debounce
	m.debouncer.Callback = func(filePath string) {
		m.eventBroadcaster.BroadcastJSON("file_detected", fmt.Sprintf("File detected: %s", filePath), map[string]interface{}{
			"path": filePath,
		})
		go m.processFile(filePath)
	}

	for _, path := range m.watchPaths {
		if err := m.addWatchPath(path); err != nil {
			log.Printf("Failed to watch path %s: %v", path, err)
			continue
		}
	}

	// Start event processing goroutines
	for path, watcher := range m.watchers {
		m.wg.Add(1)
		go m.processEvents(path, watcher)
	}

	return nil
}

// Stop stops all watchers
func (m *Manager) Stop() {
	m.cancel()
	m.debouncer.Stop()
	m.mu.Lock()
	defer m.mu.Unlock()

	for path, watcher := range m.watchers {
		if err := watcher.Close(); err != nil {
			log.Printf("Error closing watcher for %s: %v", path, err)
		}
		delete(m.watchers, path)
	}

	m.wg.Wait()

	// Close database connection
	if m.clientDB != nil {
		if err := m.clientDB.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}
}

// Reload reloads watchers with new paths
func (m *Manager) Reload(newPaths []string) error {
	m.Stop()

	m.mu.Lock()
	m.watchPaths = newPaths
	m.watchers = make(map[string]*fsnotify.Watcher)
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	m.mu.Unlock()

	return m.Start(context.Background())
}

// Status returns current status
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths := make([]string, 0, len(m.watchers))
	for path := range m.watchers {
		paths = append(paths, path)
	}

	return Status{
		WatchingPaths: paths,
	}
}

// addWatchPath adds a directory to watch (recursively)
func (m *Manager) addWatchPath(rootPath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if already watching
	if _, exists := m.watchers[absPath]; exists {
		return nil
	}

	// Create directory if it doesn't exist
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		log.Printf("Created watch directory: %s", absPath)
	}

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Recursively add all subdirectories
	if err := filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				log.Printf("Warning: failed to watch %s: %v", path, err)
			}
		}
		return nil
	}); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	m.watchers[absPath] = watcher
	log.Printf("Watching directory (recursive): %s", absPath)

	// Process existing files
	go m.processExistingFiles(absPath)

	return nil
}

// processEvents processes file system events
func (m *Manager) processEvents(path string, watcher *fsnotify.Watcher) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Handle new directories
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					// Add new directory to watcher
					if err := watcher.Add(event.Name); err != nil {
						log.Printf("Failed to watch new directory %s: %v", event.Name, err)
					} else {
						log.Printf("Added new directory to watch: %s", event.Name)
					}
				}
			}

			// Handle file changes
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Skip temporary files
				if parser.IsTemporaryFile(event.Name) {
					continue
				}
				// Check if file type is supported
				if parser.IsSupportedFile(event.Name) {
					m.eventBroadcaster.BroadcastJSON("file_detected", fmt.Sprintf("File detected: %s", event.Name), map[string]interface{}{
						"path": event.Name,
					})
					go m.processFile(event.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error for %s: %v", path, err)
			m.eventBroadcaster.BroadcastJSON("file_error", fmt.Sprintf("Watcher error: %v", err), nil)
		}
	}
}

// processExistingFiles processes files that already exist in the directory
// Uses debouncer to avoid immediate processing of all files at startup
func (m *Manager) processExistingFiles(dir string) {
	log.Printf("Scanning existing files in %s", dir)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Skip temporary files
			if parser.IsTemporaryFile(path) {
				return nil
			}
			// Process supported file types (use debouncer to batch process)
			if parser.IsSupportedFile(path) {
				m.debouncer.Trigger(path)
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Error scanning directory %s: %v", dir, err)
	}
}

// processFile processes a single file using the decision engine
func (m *Manager) processFile(filePath string) {
	// Use decision engine to determine if we should process this file
	decision, err := m.decisionEngine.Decide(filePath)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to decide on file %s: %v", filePath, err)
		log.Printf(errorMsg)
		m.eventBroadcaster.BroadcastJSON("file_error", errorMsg, map[string]interface{}{
			"path":  filePath,
			"error": err.Error(),
		})
		return
	}

	if !decision.ShouldProcess {
		log.Printf("Skipping file: %s - %s", filePath, decision.Reason)
		m.eventBroadcaster.BroadcastJSON("file_skipped", decision.Reason, map[string]interface{}{
			"path": filePath,
		})
		return
	}

	m.eventBroadcaster.BroadcastJSON("file_processing", fmt.Sprintf("Processing: %s (%s)", filePath, decision.IngestType), map[string]interface{}{
		"path": filePath,
	})

	fileType := filepath.Ext(filePath)
	log.Printf("Processing %s file: %s (type: %s, hash: %s)", fileType, filePath, decision.IngestType, decision.FileHash)

	// Extract text using the parser dispatcher
	text, err := parser.ParseFile(filePath)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to extract text from %s: %v", filePath, err)
		log.Printf(errorMsg)
		m.eventBroadcaster.BroadcastJSON("file_error", errorMsg, map[string]interface{}{
			"path":  filePath,
			"error": err.Error(),
		})
		// Mark as failed in database
		m.decisionEngine.MarkProcessed(decision, "failed")
		return
	}

	if text == "" {
		log.Printf("No text extracted from %s", filePath)
		m.decisionEngine.MarkProcessed(decision, "no_content")
		return
	}

	// Chunk the text
	chunks, err := m.chunker.ChunkText(text)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to chunk text from %s: %v", filePath, err)
		log.Printf(errorMsg)
		m.eventBroadcaster.BroadcastJSON("file_error", errorMsg, map[string]interface{}{
			"path":  filePath,
			"error": err.Error(),
		})
		m.decisionEngine.MarkProcessed(decision, "chunk_failed")
		return
	}

	log.Printf("Extracted %d chunks from %s", len(chunks), filePath)

	// Send chunks to Hive (if client is available)
	if m.droneClient == nil {
		log.Printf("Skipping ingestion for %s: No Hive server configured", filePath)
		m.eventBroadcaster.BroadcastJSON("file_error", fmt.Sprintf("No Hive server configured. File processed but not ingested: %s", filePath), map[string]interface{}{
			"path": filePath,
		})
		m.decisionEngine.MarkProcessed(decision, "no_server")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	documentID := filepath.Base(filePath)
	successCount := 0
	serverStatus := "success"

	// Prepare metadata with file hash, ingest type, and client_id
	metadata := map[string]string{
		"filename":    filepath.Base(filePath),
		"path":        filePath,
		"file_path":   filePath, // Ensure file_path is set for UUID generation
		"filetype":    fileType,
		"file_hash":   decision.FileHash,
		"ingest_type": string(decision.IngestType),
		"client_id":   m.clientID,
	}

	for i, chunk := range chunks {
		err := m.droneClient.IngestChunk(ctx, documentID, chunk, i, metadata)
		if err != nil {
			log.Printf("Failed to ingest chunk %d from %s: %v", i, filePath, err)
			serverStatus = "partial"
			continue
		}
		successCount++
	}

	// Update database with processing result
	if successCount == len(chunks) {
		serverStatus = "success"
		log.Printf("Successfully processed %s (%d chunks, type: %s)", filePath, len(chunks), decision.IngestType)
		m.eventBroadcaster.BroadcastJSON("file_complete", fmt.Sprintf("Successfully processed: %s", filePath), map[string]interface{}{
			"path":   filePath,
			"chunks": len(chunks),
		})
	} else if successCount > 0 {
		serverStatus = "partial"
		log.Printf("Partially processed %s (%d/%d chunks)", filePath, successCount, len(chunks))
		m.eventBroadcaster.BroadcastJSON("file_error", fmt.Sprintf("Partially processed %s", filePath), map[string]interface{}{
			"path":   filePath,
			"chunks": successCount,
		})
	} else {
		serverStatus = "failed"
		log.Printf("Failed to process %s (0 chunks ingested)", filePath)
		m.eventBroadcaster.BroadcastJSON("file_error", fmt.Sprintf("Failed to process %s", filePath), map[string]interface{}{
			"path": filePath,
		})
	}

	// Mark as processed in database
	if err := m.decisionEngine.MarkProcessed(decision, serverStatus); err != nil {
		log.Printf("Failed to update database for %s: %v", filePath, err)
	}
}
