// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// TrafficLogger creates a middleware that logs HTTP request entry and exit
func TrafficLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Skip logging for polling endpoints to reduce noise
		skipPaths := []string{"/api/v1/stats", "/api/v1/health", "/api/v1/keys"}
		shouldLog := true
		for _, path := range skipPaths {
			if strings.HasPrefix(r.URL.Path, path) {
				shouldLog = false
				break
			}
		}
		
		// Log request entry (only for non-polling endpoints)
		if shouldLog {
			log.Printf("[HTTP] -> %s %s", r.Method, r.URL.Path)
		}
		
		// Wrap ResponseWriter to capture status code
		// Preserve Flusher interface for SSE streams
		var rw http.ResponseWriter
		if flusher, ok := w.(http.Flusher); ok {
			rw = &responseWriterWithFlush{
				responseWriter: responseWriter{ResponseWriter: w, statusCode: http.StatusOK},
				Flusher:        flusher,
			}
		} else {
			rw = &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		}
		
		// Call next handler
		next.ServeHTTP(rw, r)
		
		// Calculate duration
		duration := time.Since(start)
		
		// Log request exit with status and duration (only for non-polling endpoints)
		if shouldLog {
			var statusCode int
			if rwWithFlush, ok := rw.(*responseWriterWithFlush); ok {
				statusCode = rwWithFlush.statusCode
			} else if rwBasic, ok := rw.(*responseWriter); ok {
				statusCode = rwBasic.statusCode
			} else {
				statusCode = http.StatusOK // Fallback
			}
			// Only log errors or slow requests for polling endpoints
			if !shouldLog && (statusCode >= 400 || duration > 1*time.Second) {
				log.Printf("[HTTP] <- %d (%s) %s %s", statusCode, duration, r.Method, r.URL.Path)
			} else if shouldLog {
				log.Printf("[HTTP] <- %d (%s) %s %s", statusCode, duration, r.Method, r.URL.Path)
			}
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// responseWriterWithFlush wraps ResponseWriter and preserves Flusher interface
type responseWriterWithFlush struct {
	responseWriter
	http.Flusher
}

func (rw *responseWriterWithFlush) Flush() {
	rw.Flusher.Flush()
}

