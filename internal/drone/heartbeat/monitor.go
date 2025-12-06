// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

// Monitor tracks server health status
type Monitor struct {
	serverURL      string
	apiKey         string
	ticker         *time.Ticker
	status         string // "up", "down", "unknown", "disabled_on_server"
	failureCount   int
	mu             sync.RWMutex
	statusCallback func(status string) // Callback to update UI
	stopChan       chan struct{}
}

// NewMonitor creates a new heartbeat monitor
func NewMonitor(serverURL, apiKey string, statusCallback func(status string)) *Monitor {
	return &Monitor{
		serverURL:      serverURL,
		apiKey:         apiKey,
		status:         "unknown",
		statusCallback: statusCallback,
		stopChan:       make(chan struct{}),
	}
}

// Start begins monitoring the server health
func (m *Monitor) Start() {
	m.ticker = time.NewTicker(10 * time.Second)

	go m.monitorLoop()
	log.Printf("Heartbeat monitor started for server: %s", m.serverURL)
}

// Stop stops the heartbeat monitor
func (m *Monitor) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	close(m.stopChan)
	log.Printf("Heartbeat monitor stopped")
}

// GetStatus returns the current server status
func (m *Monitor) GetStatus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// monitorLoop runs the monitoring loop
func (m *Monitor) monitorLoop() {
	for {
		select {
		case <-m.stopChan:
			return
		case <-m.ticker.C:
			m.checkHealth()
		}
	}
}

// checkHealth pings the server health endpoint
func (m *Monitor) checkHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	healthURL := fmt.Sprintf("%s/api/v1/health", m.serverURL)
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		log.Printf("Heartbeat: Failed to create request: %v", err)
		m.handleFailure()
		return
	}

	// Add API key to header if available
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		m.handleFailure()
		return
	}
	defer resp.Body.Close()

	// Check for disabled key response (401 with key_disabled error)
	if resp.StatusCode == http.StatusUnauthorized {
		var errorResponse struct {
			Error  string `json:"error"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err == nil {
			if errorResponse.Error == "key_disabled" || errorResponse.Status == "key_disabled" {
				m.handleKeyDisabled()
				return
			}
		}
		// If not key_disabled, treat as regular failure
		m.handleFailure()
		return
	}

	if resp.StatusCode == http.StatusOK {
		var healthResponse struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&healthResponse); err == nil {
			m.handleSuccess()
		} else {
			m.handleFailure()
		}
	} else {
		m.handleFailure()
	}
}

// handleSuccess handles a successful health check
func (m *Monitor) handleSuccess() {
	m.mu.Lock()
	wasDown := m.status == "down"
	m.status = "up"
	m.failureCount = 0
	m.mu.Unlock()

	if wasDown {
		log.Printf("Server is now reachable: %s", m.serverURL)
	}

	if m.statusCallback != nil {
		m.statusCallback("up")
	}
}

// handleKeyDisabled handles when the API key is disabled on the server
func (m *Monitor) handleKeyDisabled() {
	m.mu.Lock()
	wasDisabled := m.status == "disabled_on_server"
	m.status = "disabled_on_server"
	m.failureCount = 0
	m.mu.Unlock()

	if !wasDisabled {
		log.Printf("API key is disabled on server: %s", m.serverURL)
	}

	if m.statusCallback != nil {
		m.statusCallback("disabled_on_server")
	}
}

// handleFailure handles a failed health check
func (m *Monitor) handleFailure() {
	m.mu.Lock()
	m.failureCount++
	failureCount := m.failureCount
	m.mu.Unlock()

	log.Printf("Server health check failed (attempt %d): %s", failureCount, m.serverURL)

	if failureCount >= 3 {
		m.mu.Lock()
		m.status = "down"
		m.mu.Unlock()

		log.Printf("Server Unreachable: %s (3 consecutive failures)", m.serverURL)

		// Trigger OS notification
		title := "Server Unreachable"
		message := fmt.Sprintf("The Hive server at %s is unreachable. Please check the server status.", m.serverURL)
		if err := beeep.Alert(title, message, ""); err != nil {
			log.Printf("Failed to send OS notification: %v", err)
		}

		if m.statusCallback != nil {
			m.statusCallback("down")
		}
	}
}
