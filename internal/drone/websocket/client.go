// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package websocket

import (
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NotificationMessage represents a notification from the server
type NotificationMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

// Client manages WebSocket connection to Hive server
type Client struct {
	serverURL string
	clientID  string
	apiKey    string
	conn      *websocket.Conn
	onMessage func(NotificationMessage)
	done      chan struct{}
	closeOnce sync.Once
}

// NewClient creates a new WebSocket client
func NewClient(serverURL, clientID, apiKey string, onMessage func(NotificationMessage)) *Client {
	return &Client{
		serverURL: serverURL,
		clientID:  clientID,
		apiKey:    apiKey,
		onMessage: onMessage,
		done:      make(chan struct{}),
	}
}

// Connect connects to the WebSocket server
func (c *Client) Connect() error {
	// Parse server URL and convert to WebSocket URL
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return err
	}

	// Convert http/https to ws/wss
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	} else if u.Scheme == "" {
		// If no scheme, assume http
		wsScheme = "ws"
		u.Scheme = "http"
	}

	// Build query parameters
	query := url.Values{}
	query.Set("client_id", c.clientID)
	if c.apiKey != "" {
		query.Set("api_key", c.apiKey)
	}

	wsURL := url.URL{
		Scheme:   wsScheme,
		Host:     u.Host,
		Path:     "/api/v1/ws",
		RawQuery: query.Encode(),
	}

	log.Printf("Connecting to WebSocket: %s", wsURL.String())

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Set Authorization header if API key is provided
	headers := make(map[string][]string)
	if c.apiKey != "" {
		headers["Authorization"] = []string{"Bearer " + c.apiKey}
	}

	conn, _, err := dialer.Dial(wsURL.String(), headers)
	if err != nil {
		return err
	}

	c.conn = conn

	// Set up pong handler to respond to server pings (keepalive)
	c.conn.SetPongHandler(func(string) error {
		// Respond to pong (server sent ping, we respond with pong)
		return nil
	})

	// Set read deadline to enable ping/pong
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	log.Printf("WebSocket connected (client_id: %s)", c.clientID)

	// Start reading messages
	go c.readMessages()

	return nil
}

// readMessages reads messages from the WebSocket connection
func (c *Client) readMessages() {
	defer func() {
		// Use Close() to ensure proper cleanup with sync.Once
		_ = c.Close()
	}()

	// Start ping ticker to keep connection alive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Channel for read operations
	readChan := make(chan error, 1)

	// Start reading in a goroutine
	go func() {
		for {
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				readChan <- err
				return
			}

			// Reset read deadline on successful read
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			var notification NotificationMessage
			if err := json.Unmarshal(message, &notification); err != nil {
				log.Printf("Failed to parse notification: %v", err)
				continue
			}

			// Call the callback
			if c.onMessage != nil {
				c.onMessage(notification)
			}
		}
	}()

	for {
		select {
		case <-pingTicker.C:
			// Send ping to server
			if c.conn != nil {
				if err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("Failed to send ping: %v", err)
					return
				}
			}
		case err := <-readChan:
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				log.Printf("WebSocket connection closed, will attempt to reconnect...")
				// Trigger reconnection
				go c.reconnect()
				return
			}
		}
	}
}

// reconnect attempts to reconnect to the server
func (c *Client) reconnect() {
	for {
		time.Sleep(5 * time.Second) // Wait 5 seconds before reconnecting
		log.Printf("Attempting to reconnect WebSocket...")
		if err := c.Connect(); err != nil {
			log.Printf("Reconnection failed: %v, will retry...", err)
			continue
		}
		log.Printf("WebSocket reconnected successfully")
		return
	}
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.done != nil {
			close(c.done)
		}
		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

// Wait blocks until the connection is closed
func (c *Client) Wait() {
	<-c.done
}
