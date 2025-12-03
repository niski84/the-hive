// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/the-hive/internal/logger"
)

// HandleLogStream streams logs via Server-Sent Events (SSE)
func HandleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to log channel
	logChan := logger.GetDefault().Subscribe()

	log.Printf("New log stream client connected")

	// Send initial connection message
	fmt.Fprintf(w, "data: Connected to log stream\n\n")
	flusher.Flush()

	// Stream logs
	for {
		select {
		case logLine, ok := <-logChan:
			if !ok {
				// Channel closed
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", logLine)
			flusher.Flush()
		case <-r.Context().Done():
			log.Printf("Log stream client disconnected")
			return
		}
	}
}

