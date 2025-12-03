// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	fmt.Println("🧪 Starting Integration Test...")

	// Step 1: Connect to WebSocket
	fmt.Println("Step 1: Connecting to WebSocket...")
	wsURL := "ws://localhost:8081/api/v1/ws?client_id=test-client-" + fmt.Sprintf("%d", time.Now().Unix())
	
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to connect to WebSocket: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("✅ Connected to WebSocket")

	// Channel to receive notifications
	notifications := make(chan map[string]interface{}, 10)
	done := make(chan struct{})

	// Read messages from WebSocket
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("WebSocket error: %v\n", err)
				}
				return
			}

			var notification map[string]interface{}
			if err := json.Unmarshal(message, &notification); err != nil {
				continue
			}

			// Check if it's an ALERT
			if notification["type"] == "ALERT" {
				select {
				case notifications <- notification:
				default:
				}
			}
		}
	}()

	// Step 2: Define a temporary rule (or use default)
	fmt.Println("Step 2: Adding test rule...")
	ruleQuery := "Does this document contain confidential pricing information?"
	
	rulePayload := map[string]interface{}{
		"query":  ruleQuery,
		"active": true,
	}

	ruleJSON, _ := json.Marshal(rulePayload)
	req, err := http.NewRequest("POST", "http://localhost:8081/api/v1/rules/add", bytes.NewBuffer(ruleJSON))
	if err != nil {
		fmt.Printf("❌ Failed to create rule request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to add rule: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Failed to add rule (status %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	fmt.Println("✅ Rule added successfully")

	// Step 3: HTTP POST a text file containing the trigger word
	fmt.Println("Step 3: Sending test document...")
	testContent := "This document contains CONFIDENTIAL pricing information for Q4 2025."
	
	ingestPayload := map[string]interface{}{
		"file_path": "test_confidential.txt",
		"content":   testContent,
		"metadata": map[string]string{
			"filename":  "test_confidential.txt",
			"filetype":  ".txt",
			"client_id": "test-client-" + fmt.Sprintf("%d", time.Now().Unix()),
		},
	}

	ingestJSON, _ := json.Marshal(ingestPayload)
	req, err = http.NewRequest("POST", "http://localhost:8081/api/v1/ingest", bytes.NewBuffer(ingestJSON))
	if err != nil {
		fmt.Printf("❌ Failed to create ingest request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to ingest document: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Failed to ingest document (status %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	fmt.Println("✅ Document ingested successfully")

	// Step 4: Listen for alert (5 second timeout)
	fmt.Println("Step 4: Waiting for alert notification (5 second timeout)...")
	timeout := time.After(5 * time.Second)

	select {
	case notification := <-notifications:
		fmt.Println("✅ ALERT received!")
		fmt.Printf("   Type: %v\n", notification["type"])
		fmt.Printf("   Message: %v\n", notification["message"])
		fmt.Printf("   Level: %v\n", notification["level"])
		fmt.Println("\n🎉 Integration test PASSED!")
		os.Exit(0)
	case <-timeout:
		fmt.Println("❌ Timeout: No alert received within 5 seconds")
		fmt.Println("   This could mean:")
		fmt.Println("   - The analyst worker is not running")
		fmt.Println("   - The rule was not triggered")
		fmt.Println("   - The WebSocket connection failed")
		os.Exit(1)
	case <-done:
		fmt.Println("❌ WebSocket connection closed unexpectedly")
		os.Exit(1)
	}
}

