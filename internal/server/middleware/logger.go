// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package middleware

import (
	"log"
	"net/http"
	"time"
)

// TrafficLogger creates a middleware that logs HTTP request entry and exit
func TrafficLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Log request entry
		log.Printf("[HTTP] -> %s %s", r.Method, r.URL.Path)
		
		// Wrap ResponseWriter to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		// Call next handler
		next.ServeHTTP(rw, r)
		
		// Calculate duration
		duration := time.Since(start)
		
		// Log request exit with status and duration
		log.Printf("[HTTP] <- %d (%s) %s %s", rw.statusCode, duration, r.Method, r.URL.Path)
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

