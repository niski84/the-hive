// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/the-hive/internal/drone"
	"github.com/the-hive/internal/drone/events"
	"github.com/the-hive/internal/drone/watcher"
)

var (
	serverStatus     string = "unknown"
	serverStatusLock sync.RWMutex
)

// UpdateServerStatus updates the server status for UI display
func UpdateServerStatus(status string) {
	serverStatusLock.Lock()
	defer serverStatusLock.Unlock()
	serverStatus = status
}

// GetServerStatus returns the current server status
func GetServerStatus() string {
	serverStatusLock.RLock()
	defer serverStatusLock.RUnlock()
	return serverStatus
}

// Server handles the web UI and API
type Server struct {
	config           *drone.Config
	watcherMgr       *watcher.Manager
	eventBroadcaster *events.Broadcaster
	uiFiles          embed.FS
	mu               sync.RWMutex
}

// NewServer creates a new web server instance
func NewServer(config *drone.Config, watcherMgr *watcher.Manager, broadcaster *events.Broadcaster, uiFiles embed.FS) *Server {
	return &Server{
		config:           config,
		watcherMgr:       watcherMgr,
		eventBroadcaster: broadcaster,
		uiFiles:          uiFiles,
	}
}

// Address returns the server address
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.config.WebServer.Port)
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Serve embedded UI files
	s.serveUI(mux)

	// API endpoints
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/save", s.handleSaveConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/server-status", s.handleServerStatus)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/watch-paths", s.handleWatchPaths)
	mux.HandleFunc("/api/watch-paths/add", s.handleAddWatchPath)
	mux.HandleFunc("/api/watch-paths/remove", s.handleRemoveWatchPath)
	mux.HandleFunc("/api/watch-paths/toggle", s.handleToggleWatchPath)
	mux.HandleFunc("/api/v1/shutdown", s.handleShutdown)

	return mux
}

// serveUI serves the embedded UI files
func (s *Server) serveUI(mux *http.ServeMux) {
	// Serve static files from embedded FS
	fsys, err := fs.Sub(s.uiFiles, "ui")
	if err != nil {
		log.Printf("Failed to create sub filesystem: %v", err)
		return
	}

	fileServer := http.FileServer(http.FS(fsys))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// Serve index.html for root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/settings" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/settings" {
			s.serveSettings(w, r)
		} else {
			s.serveIndex(w, r)
		}
	})
}

// serveIndex serves the main dashboard page
func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	indexHTML, err := s.uiFiles.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "Failed to load UI", http.StatusInternalServerError)
		return
	}

	// Parse and execute template
	tmpl, err := template.New("index").Parse(string(indexHTML))
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// serveSettings serves the settings page
func (s *Server) serveSettings(w http.ResponseWriter, r *http.Request) {
	settingsHTML, err := s.uiFiles.ReadFile("ui/settings.html")
	if err != nil {
		http.Error(w, "Failed to load settings page", http.StatusInternalServerError)
		return
	}

	// Parse and execute template
	tmpl, err := template.New("settings").Parse(string(settingsHTML))
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// handleConfig returns the current configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	config := s.config
	watchPaths := config.WatchPaths
	disabledPaths := config.DisabledPaths
	s.mu.RUnlock()

	// Validate all paths and add validation status
	pathStatus := make(map[string]map[string]interface{})
	for _, path := range watchPaths {
		isValid, err := validatePath(path)
		isDisabled := false
		for _, disabled := range disabledPaths {
			if path == disabled {
				isDisabled = true
				break
			}
		}
		
		pathStatus[path] = map[string]interface{}{
			"valid":    isValid,
			"disabled": isDisabled,
		}
		if !isValid {
			pathStatus[path]["error"] = err.Error()
		}
	}

	// Create response with path validation
	response := map[string]interface{}{
		"Server":          config.Server,
		"GrpcServerAddress": config.GrpcServerAddress,
		"APIKey":          config.APIKey,
		"WatchPaths":      config.WatchPaths,
		"DisabledPaths":   config.DisabledPaths,
		"WebServer":       config.WebServer,
		"PathStatus":      pathStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSaveConfig saves configuration changes
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newConfig drone.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Update config
	s.mu.Lock()
	if newConfig.Server.Address != "" {
		s.config.Server.Address = newConfig.Server.Address
	}
	if newConfig.GrpcServerAddress != "" {
		s.config.GrpcServerAddress = newConfig.GrpcServerAddress
	}
	if newConfig.APIKey != "" {
		s.config.APIKey = newConfig.APIKey
	}
	if len(newConfig.WatchPaths) > 0 {
		s.config.WatchPaths = newConfig.WatchPaths
	}
	if len(newConfig.DisabledPaths) >= 0 {
		s.config.DisabledPaths = newConfig.DisabledPaths
	}
	if newConfig.WebServer.Port > 0 {
		s.config.WebServer.Port = newConfig.WebServer.Port
	}
	config := s.config
	s.mu.Unlock()

	// Save to file
	configPath := ""
	if r.URL.Query().Get("config") != "" {
		configPath = r.URL.Query().Get("config")
	}
	if err := drone.SaveConfig(config, configPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// Reload watcher with new paths
	if err := s.watcherMgr.Reload(config.WatchPaths, config.DisabledPaths); err != nil {
		log.Printf("Failed to reload watcher: %v", err)
		http.Error(w, fmt.Sprintf("Config saved but watcher reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// validatePath checks if a path is valid and accessible
func validatePath(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("path is empty")
	}
	
	// Try to resolve absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("invalid path format: %w", err)
	}
	
	// Check if path exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("path does not exist")
		}
		return false, fmt.Errorf("cannot access path: %w", err)
	}
	
	// Check if it's a directory (we only watch directories)
	if !info.IsDir() {
		return false, fmt.Errorf("path is not a directory")
	}
	
	return true, nil
}

// handleStatus returns current status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.watcherMgr.Status()
	
	// Get all configured paths and validate them
	s.mu.RLock()
	allPaths := s.config.WatchPaths
	disabledPaths := s.config.DisabledPaths
	s.mu.RUnlock()
	
	// Create a map of path validation status
	pathStatus := make(map[string]map[string]interface{})
	for _, path := range allPaths {
		isValid, err := validatePath(path)
		isDisabled := false
		for _, disabled := range disabledPaths {
			if path == disabled {
				isDisabled = true
				break
			}
		}
		
		pathStatus[path] = map[string]interface{}{
			"valid":    isValid,
			"disabled": isDisabled,
		}
		if !isValid {
			pathStatus[path]["error"] = err.Error()
		}
	}
	
	// Add path validation to status
	statusWithValidation := map[string]interface{}{
		"watching_paths": status.WatchingPaths,
		"total_files":    status.TotalFiles,
		"processed":      status.Processed,
		"errors":         status.Errors,
		"path_status":    pathStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusWithValidation)
}

// handleServerStatus returns the Hive server connection status
func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	serverAddr := s.config.Server.Address
	s.mu.RUnlock()

	// Check if server URL is empty
	if serverAddr == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "config_required"})
		return
	}

	status := GetServerStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

// handleStream handles Server-Sent Events
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	clientChan := make(chan events.Event, 10)
	s.eventBroadcaster.Subscribe(clientChan)
	defer s.eventBroadcaster.Unsubscribe(clientChan)

	// Send initial connection message
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"connected","message":"Connected to event stream"}`)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Send events as they come
	for {
		select {
		case event := <-clientChan:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// handleWatchPaths returns current watch paths
func (s *Server) handleWatchPaths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	paths := s.config.WatchPaths
	disabledPaths := s.config.DisabledPaths
	s.mu.RUnlock()

	// Create a map of disabled paths for quick lookup
	disabledMap := make(map[string]bool)
	for _, p := range disabledPaths {
		disabledMap[p] = true
	}

	// Build response with enabled status
	pathList := []map[string]interface{}{}
	for _, path := range paths {
		pathList = append(pathList, map[string]interface{}{
			"path":    path,
			"enabled": !disabledMap[path],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"paths": pathList})
}

// handleAddWatchPath adds a new watch path
func (s *Server) handleAddWatchPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	// Check if path already exists
	for _, p := range s.config.WatchPaths {
		if p == req.Path {
			s.mu.Unlock()
			http.Error(w, "Path already watched", http.StatusBadRequest)
			return
		}
	}
	s.config.WatchPaths = append(s.config.WatchPaths, req.Path)
	config := s.config
	s.mu.Unlock()

	// Reload watcher
	if err := s.watcherMgr.Reload(config.WatchPaths, config.DisabledPaths); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add watch path: %v", err), http.StatusInternalServerError)
		return
	}

	// Save config
	if err := drone.SaveConfig(config, ""); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleRemoveWatchPath removes a watch path
func (s *Server) handleRemoveWatchPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	newPaths := []string{}
	for _, p := range s.config.WatchPaths {
		if p != req.Path {
			newPaths = append(newPaths, p)
		}
	}
	s.config.WatchPaths = newPaths
	config := s.config
	s.mu.Unlock()

	// Reload watcher
	if err := s.watcherMgr.Reload(config.WatchPaths, config.DisabledPaths); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove watch path: %v", err), http.StatusInternalServerError)
		return
	}

	// Save config
	if err := drone.SaveConfig(config, ""); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleToggleWatchPath toggles a watch path between enabled and disabled
func (s *Server) handleToggleWatchPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	// Toggle the path
	if err := s.watcherMgr.TogglePath(req.Path, req.Enabled); err != nil {
		http.Error(w, fmt.Sprintf("Failed to toggle path: %v", err), http.StatusInternalServerError)
		return
	}

	// Update config
	s.mu.Lock()
	// Update disabled paths in config
	newDisabled := []string{}
	for _, p := range s.config.DisabledPaths {
		if p != req.Path {
			newDisabled = append(newDisabled, p)
		}
	}
	if !req.Enabled {
		// Add to disabled if not already there
		found := false
		for _, p := range newDisabled {
			if p == req.Path {
				found = true
				break
			}
		}
		if !found {
			newDisabled = append(newDisabled, req.Path)
		}
	}
	s.config.DisabledPaths = newDisabled
	config := s.config
	s.mu.Unlock()

	// Save config
	if err := drone.SaveConfig(config, ""); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleShutdown handles POST /api/v1/shutdown requests
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("Shutting down request received...")

	// Notify Hive server that this client is shutting down
	s.mu.RLock()
	serverAddr := s.config.Server.Address
	apiKey := s.config.APIKey
	s.mu.RUnlock()

	// If server address and API key are configured, notify the server
	if serverAddr != "" && apiKey != "" {
		// Convert gRPC address to HTTP address if needed
		serverURL := serverAddr
		if !strings.Contains(serverURL, "://") {
			serverURL = "http://" + strings.Replace(serverURL, ":50051", ":8081", 1)
		}

		// Call server shutdown notification endpoint
		shutdownURL := fmt.Sprintf("%s/api/v1/client/shutdown", serverURL)
		req, err := http.NewRequest("POST", shutdownURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/json")
			
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				log.Printf("Notified Hive server of client shutdown")
			} else {
				log.Printf("Failed to notify Hive server of shutdown: %v", err)
			}
		} else {
			log.Printf("Failed to create shutdown notification request: %v", err)
		}
	}

	// Send response before shutting down
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "shutting down"})

	// Flush the response
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Wait 1 second then exit
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("Client shutdown complete")
		os.Exit(0)
	}()
}
