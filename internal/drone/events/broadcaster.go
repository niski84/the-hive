// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package events

import (
	"sync"
	"time"
)

// Event represents a file processing event
type Event struct {
	Type      string    `json:"type"`      // "file_detected", "file_processing", "file_complete", "file_error"
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path,omitempty"`
	Message   string    `json:"message"`
	Chunks    int       `json:"chunks,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Broadcaster manages SSE client subscriptions
type Broadcaster struct {
	subscribers map[chan Event]bool
	mu          sync.RWMutex
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[chan Event]bool),
	}
}

// Subscribe adds a new subscriber
func (eb *Broadcaster) Subscribe(ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.subscribers[ch] = true
}

// Unsubscribe removes a subscriber
func (eb *Broadcaster) Unsubscribe(ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.subscribers, ch)
	close(ch)
}

// Broadcast sends an event to all subscribers
func (eb *Broadcaster) Broadcast(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber
		}
	}
}

// BroadcastJSON broadcasts a JSON message directly
func (eb *Broadcaster) BroadcastJSON(eventType, message string, data map[string]interface{}) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Message:   message,
	}

	if data != nil {
		if path, ok := data["path"].(string); ok {
			event.Path = path
		}
		if chunks, ok := data["chunks"].(int); ok {
			event.Chunks = chunks
		}
		if err, ok := data["error"].(string); ok {
			event.Error = err
		}
	}

	eb.Broadcast(event)
}

