// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package watcher

import (
	"sync"
	"time"
)

// Debouncer manages debouncing of file system events
type Debouncer struct {
	mu       sync.Mutex
	timers   map[string]*time.Timer
	Callback func(string) // Exported so it can be set from Manager
	delay    time.Duration
}

// NewDebouncer creates a new debouncer with the specified delay
func NewDebouncer(delay time.Duration, callback func(string)) *Debouncer {
	return &Debouncer{
		timers:   make(map[string]*time.Timer),
		Callback: callback,
		delay:    delay,
	}
}

// Trigger schedules or resets the timer for a file path
func (d *Debouncer) Trigger(filePath string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel existing timer if any
	if timer, exists := d.timers[filePath]; exists {
		timer.Stop()
	}

	// Create new timer
	d.timers[filePath] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, filePath)
		callback := d.Callback
		d.mu.Unlock()

		// Call the callback
		if callback != nil {
			callback(filePath)
		}
	})
}

// Cancel cancels any pending timer for a file path
func (d *Debouncer) Cancel(filePath string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if timer, exists := d.timers[filePath]; exists {
		timer.Stop()
		delete(d.timers, filePath)
	}
}

// Stop cancels all pending timers
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, timer := range d.timers {
		timer.Stop()
	}
	d.timers = make(map[string]*time.Timer)
}

