// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/the-hive/internal/drone"
	"github.com/the-hive/internal/drone/events"
	"github.com/the-hive/internal/drone/watcher"
	"github.com/the-hive/internal/drone/web"
)

//go:embed ui/*
var uiFiles embed.FS

var (
	configPath = flag.String("config", "", "Path to config file (default: ~/.the-hive/config.yaml)")
	serverAddr = flag.String("server", "", "Hive server address (overrides config)")
	watchDirs  = flag.String("watch-dirs", "", "Comma-separated list of directories to watch (overrides config)")
	webPort    = flag.Int("web-port", 0, "Web server port (overrides config)")
)

func main() {
	flag.Parse()

	// Load configuration
	config, err := drone.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Apply CLI flag overrides
	watchDirList := []string{}
	if *watchDirs != "" {
		// Parse comma-separated directories
		for _, dir := range strings.Split(*watchDirs, ",") {
			dir = strings.TrimSpace(dir)
			if dir != "" {
				watchDirList = append(watchDirList, dir)
			}
		}
	}
	drone.ApplyCLIFlags(config, *serverAddr, watchDirList, *webPort)

	log.Printf("Loaded configuration:")
	log.Printf("  Server: %s", config.Server.Address)
	log.Printf("  Watch paths: %v", config.WatchPaths)
	log.Printf("  Web server port: %d", config.WebServer.Port)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create event broadcaster for SSE
	eventBroadcaster := events.NewBroadcaster()

	// Get config directory for database
	configDir := ""
	if *configPath != "" {
		configDir = filepath.Dir(*configPath)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			configDir = filepath.Join(home, ".the-hive")
		} else {
			configDir = "./.the-hive"
		}
	}

	// Initialize file watcher manager with database support
	watcherMgr, err := watcher.NewManager(config.WatchPaths, config.Server.Address, eventBroadcaster, configDir)
	if err != nil {
		log.Fatalf("Failed to initialize watcher manager: %v", err)
	}

	// Start file watcher
	if err := watcherMgr.Start(ctx); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcherMgr.Stop()

	// Initialize web server
	webServer := web.NewServer(config, watcherMgr, eventBroadcaster, uiFiles)

	// Start web server
	httpServer := &http.Server{
		Addr:    webServer.Address(),
		Handler: webServer.Handler(),
	}

	go func() {
		log.Printf("Web server starting on http://%s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Web server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("Drone client running. Press Ctrl+C to stop.")
	<-sigChan

	log.Printf("Shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down web server: %v", err)
	}

	cancel()
	log.Printf("Shutdown complete")
}
