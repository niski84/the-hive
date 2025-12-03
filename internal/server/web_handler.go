// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"embed"
	"html/template"
	"log"
	"net/http"
)

//go:embed templates/*
var templatesFS embed.FS

// HandleWeb serves the main web interface
func HandleWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse template from embedded filesystem
	tmpl, err := template.ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleSettings serves the settings page
func HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse template from embedded filesystem
	tmpl, err := template.ParseFS(templatesFS, "templates/settings.html")
	if err != nil {
		log.Printf("Failed to parse settings template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Failed to execute settings template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

