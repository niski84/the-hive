// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"fmt"
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
	loggerInstance := logger.GetDefault()
	if loggerInstance == nil {
		logger.Errorf("[ERROR] Logger instance is nil in HandleLogStream")
		http.Error(w, "Logger not initialized", http.StatusInternalServerError)
		return
	}

	// Create a per-client channel (like the client's broadcaster pattern)
	clientChan, unsubscribeChan := loggerInstance.Subscribe()
	if clientChan == nil {
		logger.Warnf("[WARN] Log channel is nil - logger may be closed")
		http.Error(w, "Log stream unavailable - logger may be closed", http.StatusInternalServerError)
		return
	}
	
	// Unsubscribe when client disconnects
	defer loggerInstance.Unsubscribe(unsubscribeChan)

	// Send initial connection message
	fmt.Fprintf(w, "data: Connected to log stream\n\n")
	flusher.Flush()

	// Stream logs from the client's channel
	for {
		select {
		case logLine, ok := <-clientChan:
			if !ok {
				// Channel closed - send a final message and return
				fmt.Fprintf(w, "data: Log stream closed\n\n")
				flusher.Flush()
				return
			}
			// Write log line to SSE stream
			if _, err := fmt.Fprintf(w, "data: %s\n\n", logLine); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

