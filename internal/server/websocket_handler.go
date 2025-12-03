// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, you should validate the origin
		return true
	},
}

// NotificationMessage represents a message sent to clients
type NotificationMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

// WebSocketManager manages WebSocket connections
type WebSocketManager struct {
	clients     map[string]*websocket.Conn
	clientsMu   sync.RWMutex
	redisClient *redis.Client
	pingTicker  *time.Ticker
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(redisClient *redis.Client) *WebSocketManager {
	ctx, cancel := context.WithCancel(context.Background())
	wm := &WebSocketManager{
		clients:     make(map[string]*websocket.Conn),
		redisClient: redisClient,
		pingTicker:  time.NewTicker(30 * time.Second),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// Start ping ticker goroutine
	go wm.pingLoop()
	
	return wm
}

// pingLoop sends ping messages to all connected clients
func (wm *WebSocketManager) pingLoop() {
	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-wm.pingTicker.C:
			wm.pingAllClients()
		}
	}
}

// pingAllClients sends ping to all connected clients and removes dead connections
func (wm *WebSocketManager) pingAllClients() {
	wm.clientsMu.RLock()
	clients := make(map[string]*websocket.Conn)
	for id, conn := range wm.clients {
		clients[id] = conn
	}
	wm.clientsMu.RUnlock()

	for clientID, conn := range clients {
		// Set write deadline
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		
		// Send ping
		if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Failed to ping client %s, removing connection: %v", clientID, err)
			// Remove dead connection
			wm.clientsMu.Lock()
			delete(wm.clients, clientID)
			wm.clientsMu.Unlock()
			conn.Close()
			continue
		}
		
		// Set read deadline to detect if client doesn't respond with pong
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
}

// HandleWebSocket handles WebSocket connections
func (wm *WebSocketManager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get client_id from query parameter
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "client_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Note: API key authentication is handled by the auth middleware before this function is called

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("WebSocket client connected: %s", clientID)

	// Add client to map
	wm.clientsMu.Lock()
	wm.clients[clientID] = conn
	wm.clientsMu.Unlock()

	// Remove client when connection closes
	defer func() {
		wm.clientsMu.Lock()
		delete(wm.clients, clientID)
		wm.clientsMu.Unlock()
		log.Printf("WebSocket client disconnected: %s", clientID)
	}()

	// Send any pending messages from Redis
	if wm.redisClient != nil {
		if err := wm.sendPendingMessages(clientID, conn); err != nil {
			log.Printf("Failed to send pending messages to %s: %v", clientID, err)
		}
	}

	// Set up pong handler to reset read deadline when ping is received
	conn.SetPongHandler(func(string) error {
		// Reset read deadline on pong (client responded to ping)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	// Set initial read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Keep connection alive and handle incoming messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", clientID, err)
			}
			break
		}

		// Reset read deadline on successful message read
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Echo back or handle message (for now, just log)
		log.Printf("Received message from client %s: %s", clientID, string(message))
	}
}

// SendNotification sends a notification to a client
// If client is online, send via WebSocket. If offline, push to Redis.
func (wm *WebSocketManager) SendNotification(clientID string, notification NotificationMessage) error {
	return wm.SendNotificationRaw(clientID, notification.Type, notification.Message, notification.Level)
}

// SendNotificationRaw sends a notification with raw parameters (implements worker.NotificationSender interface)
func (wm *WebSocketManager) SendNotificationRaw(clientID string, notificationType, message, level string) error {
	notification := NotificationMessage{
		Type:    notificationType,
		Message: message,
		Level:   level,
	}
	// Try to send via WebSocket first
	wm.clientsMu.RLock()
	conn, online := wm.clients[clientID]
	wm.clientsMu.RUnlock()

	if online && conn != nil {
		// Client is online, send via WebSocket
		messageJSON, err := json.Marshal(notification)
		if err != nil {
			return err
		}

		if err := conn.WriteMessage(websocket.TextMessage, messageJSON); err != nil {
			log.Printf("Failed to send WebSocket message to %s: %v", clientID, err)
			// Fall through to Redis fallback
			online = false
		} else {
			log.Printf("Sent notification to client %s via WebSocket", clientID)
			return nil
		}
	}

	// Client is offline, push to Redis
	if wm.redisClient != nil {
		messageJSON, err := json.Marshal(notification)
		if err != nil {
			return err
		}

		mailboxKey := "mailbox:" + clientID
		if err := wm.redisClient.LPush(context.Background(), mailboxKey, messageJSON).Err(); err != nil {
			return err
		}

		// Set expiration on mailbox (e.g., 7 days)
		wm.redisClient.Expire(context.Background(), mailboxKey, 7*24*60*60*1000000000) // 7 days in nanoseconds

		log.Printf("Queued notification for offline client %s in Redis", clientID)
		return nil
	}

	return nil
}

// sendPendingMessages sends any pending messages from Redis to the client
func (wm *WebSocketManager) sendPendingMessages(clientID string, conn *websocket.Conn) error {
	if wm.redisClient == nil {
		return nil
	}

	mailboxKey := "mailbox:" + clientID
	ctx := context.Background()

	// Pop all messages from the mailbox
	for {
		result, err := wm.redisClient.RPop(ctx, mailboxKey).Result()
		if err == redis.Nil {
			// No more messages
			break
		}
		if err != nil {
			return err
		}

		// Send message to client
		if err := conn.WriteMessage(websocket.TextMessage, []byte(result)); err != nil {
			log.Printf("Failed to send pending message to client %s: %v", clientID, err)
			// Put message back at the front of the queue
			wm.redisClient.LPush(ctx, mailboxKey, result)
			return err
		}

		log.Printf("Sent pending message to client %s", clientID)
	}

	return nil
}

// Stop stops the ping ticker and cleans up resources
func (wm *WebSocketManager) Stop() {
	wm.cancel()
	if wm.pingTicker != nil {
		wm.pingTicker.Stop()
	}
	
	// Close all connections
	wm.clientsMu.Lock()
	for clientID, conn := range wm.clients {
		conn.Close()
		delete(wm.clients, clientID)
	}
	wm.clientsMu.Unlock()
	
	log.Printf("WebSocket manager stopped")
}
