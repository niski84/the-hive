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

// renderTemplate is a helper function to render templates with base layout
func renderTemplate(w http.ResponseWriter, tmplName string, data interface{}) error {
	// Parse both base.html and the requested template together
	tmpl, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+tmplName)
	if err != nil {
		log.Printf("Failed to parse template %s: %v", tmplName, err)
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("Failed to execute template %s: %v", tmplName, err)
		return err
	}
	return nil
}

// HandleWeb serves the main web interface (search page)
func HandleWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := renderTemplate(w, "index.html", nil); err != nil {
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

	if err := renderTemplate(w, "settings.html", nil); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleTimelinePage serves the timeline visualization page
func HandleTimelinePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := renderTemplate(w, "timeline.html", nil); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleGraphPage serves the graph visualization page
func HandleGraphPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := renderTemplate(w, "graph.html", nil); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}


