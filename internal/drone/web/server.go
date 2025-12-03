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
	"sync"

	"github.com/the-hive/internal/drone"
	"github.com/the-hive/internal/drone/events"
	"github.com/the-hive/internal/drone/watcher"
)

// Server handles the web UI and API
type Server struct {
	config          *drone.Config
	watcherMgr      *watcher.Manager
	eventBroadcaster *events.Broadcaster
	uiFiles         embed.FS
	mu              sync.RWMutex
}

// NewServer creates a new web server instance
func NewServer(config *drone.Config, watcherMgr *watcher.Manager, broadcaster *events.Broadcaster, uiFiles embed.FS) *Server {
	return &Server{
		config:          config,
		watcherMgr:      watcherMgr,
		eventBroadcaster: broadcaster,
		uiFiles:         uiFiles,
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
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/watch-paths", s.handleWatchPaths)
	mux.HandleFunc("/api/watch-paths/add", s.handleAddWatchPath)
	mux.HandleFunc("/api/watch-paths/remove", s.handleRemoveWatchPath)

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
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.serveIndex(w, r)
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

// handleConfig returns the current configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
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
	if len(newConfig.WatchPaths) > 0 {
		s.config.WatchPaths = newConfig.WatchPaths
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
	if err := s.watcherMgr.Reload(config.WatchPaths); err != nil {
		log.Printf("Failed to reload watcher: %v", err)
		http.Error(w, fmt.Sprintf("Config saved but watcher reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleStatus returns current status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.watcherMgr.Status()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
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
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"paths": paths})
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
	if err := s.watcherMgr.Reload(config.WatchPaths); err != nil {
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
	if err := s.watcherMgr.Reload(config.WatchPaths); err != nil {
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

