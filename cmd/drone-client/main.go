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

	"github.com/gen2brain/beeep"
	"github.com/the-hive/internal/drone"
	"github.com/the-hive/internal/drone/events"
	"github.com/the-hive/internal/drone/heartbeat"
	"github.com/the-hive/internal/drone/watcher"
	"github.com/the-hive/internal/drone/web"
	wsclient "github.com/the-hive/internal/drone/websocket"
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
	log.Printf("  Client ID: %s", config.ClientID)
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
	watcherMgr, err := watcher.NewManager(config.WatchPaths, config.DisabledPaths, config.Server.Address, config.GrpcServerAddress, config.ClientID, eventBroadcaster, configDir)
	if err != nil {
		log.Fatalf("Failed to initialize watcher manager: %v", err)
	}

	// Start file watcher
	if err := watcherMgr.Start(ctx); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}
	// Ensure database is closed at the end of main (after watcher stops)
	defer func() {
		watcherMgr.Stop() // This will close the database
	}()

	// Initialize WebSocket client if server address is configured
	var wsClient *wsclient.Client
	if config.Server.Address != "" {
		// Convert gRPC address to HTTP address for WebSocket
		serverURL := config.Server.Address
		if !strings.Contains(serverURL, "://") {
			// Assume it's a gRPC address, convert to HTTP
			serverURL = "http://" + strings.Replace(serverURL, ":50051", ":8081", 1)
		}

		wsClient = wsclient.NewClient(serverURL, config.ClientID, config.APIKey, func(notification wsclient.NotificationMessage) {
			log.Printf("🔔 [%s] %s: %s", notification.Level, notification.Type, notification.Message)
			// Broadcast notification to UI via event broadcaster
			eventBroadcaster.BroadcastJSON("notification", notification.Message, map[string]interface{}{
				"type":  notification.Type,
				"level": notification.Level,
			})
			
			// Trigger OS notification for rule matches (ALERT type)
			if notification.Type == "ALERT" {
				title := "The Hive - Rule Match"
				message := notification.Message
				if err := beeep.Alert(title, message, ""); err != nil {
					log.Printf("Failed to send OS notification: %v", err)
				}
			}
		})

		// Connect WebSocket in background
		go func() {
			if err := wsClient.Connect(); err != nil {
				log.Printf("Failed to connect WebSocket: %v (will retry)", err)
			}
		}()
	}

	// Initialize heartbeat monitor
	var statusCallback func(string)
	statusCallback = func(status string) {
		web.UpdateServerStatus(status)
	}

	var heartbeatMonitor *heartbeat.Monitor
	if config.Server.Address != "" {
		// Convert gRPC address to HTTP address for health check
		serverURL := config.Server.Address
		if !strings.Contains(serverURL, "://") {
			// Assume it's a gRPC address, convert to HTTP
			serverURL = "http://" + strings.Replace(serverURL, ":50051", ":8081", 1)
		}

		heartbeatMonitor = heartbeat.NewMonitor(serverURL, config.APIKey, statusCallback)
		heartbeatMonitor.Start()
		defer heartbeatMonitor.Stop()
	}

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

	// Close WebSocket connection
	if wsClient != nil {
		wsClient.Close()
	}

	cancel()
	log.Printf("Shutdown complete")
}
