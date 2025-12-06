// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// Logger wraps the standard log package with file output and broadcasting
type Logger struct {
	file       *os.File
	logger     *log.Logger
	broadcast  chan string
	subscribers map[chan string]bool
	subMu      sync.RWMutex
	mu         sync.RWMutex
	closed     bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger
// If already initialized, returns the existing logger (even if closed)
func Init(logFile string) (*Logger, error) {
	var err error
	once.Do(func() {
		defaultLogger, err = NewLogger(logFile)
	})
	return defaultLogger, err
}

// NewLogger creates a new logger instance
func NewLogger(logFile string) (*Logger, error) {
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer: stdout + file
	multiWriter := io.MultiWriter(os.Stdout, file)

	logger := &Logger{
		file:        file,
		logger:      log.New(multiWriter, "", log.LstdFlags|log.Lshortfile),
		broadcast:   make(chan string, 100), // Buffered channel to prevent blocking
		subscribers: make(map[chan string]bool),
		closed:      false,
	}
	
	// Start broadcaster goroutine
	go logger.broadcastLoop()

	return logger, nil
}

// GetDefault returns the default logger instance
// If the logger is closed, it creates a new fallback logger
func GetDefault() *Logger {
	if defaultLogger == nil {
		// Fallback to stdout-only logger if not initialized
		defaultLogger = &Logger{
			logger:      log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
			broadcast:   make(chan string, 100),
			subscribers: make(map[chan string]bool),
			closed:      false, // Explicitly set closed to false
		}
		go defaultLogger.broadcastLoop()
		return defaultLogger
	}
	
	// Check if the logger is closed or if broadcast channel is closed
	defaultLogger.mu.RLock()
	closed := defaultLogger.closed
	broadcast := defaultLogger.broadcast
	defaultLogger.mu.RUnlock()
	
	if closed || broadcast == nil {
		// Logger was closed - create a new fallback logger
		// This ensures we always have a working logger even if the original was closed
		defaultLogger = &Logger{
			logger:      log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
			broadcast:   make(chan string, 100),
			subscribers: make(map[chan string]bool),
			closed:      false,
		}
		go defaultLogger.broadcastLoop()
	}
	
	return defaultLogger
}

// Subscribe creates a new channel for this client and subscribes it
// Returns the channel that will receive log messages
// If the logger is closed, returns nil
// Also returns the bidirectional channel for unsubscribe
func (l *Logger) Subscribe() (<-chan string, chan string) {
	if l == nil {
		return nil, nil
	}
	
	l.mu.RLock()
	closed := l.closed
	broadcast := l.broadcast
	l.mu.RUnlock()
	
	if closed || broadcast == nil {
		return nil, nil
	}
	
	// Create a per-client channel (like the client's broadcaster pattern)
	clientChan := make(chan string, 10)
	
	l.subMu.Lock()
	if l.subscribers == nil {
		l.subscribers = make(map[chan string]bool)
	}
	l.subscribers[clientChan] = true
	l.subMu.Unlock()
	
	return clientChan, clientChan
}

// Unsubscribe removes a client channel from subscribers
func (l *Logger) Unsubscribe(ch chan string) {
	if ch == nil {
		return
	}
	
	l.subMu.Lock()
	defer l.subMu.Unlock()
	
	if l.subscribers[ch] {
		delete(l.subscribers, ch)
		close(ch)
	}
}

// broadcastLoop reads from the main broadcast channel and forwards to all subscribers
func (l *Logger) broadcastLoop() {
	defer func() {
		// Clean up all subscribers if broadcast loop exits
		l.subMu.Lock()
		for ch := range l.subscribers {
			close(ch)
		}
		l.subscribers = make(map[chan string]bool)
		l.subMu.Unlock()
	}()
	
	for logLine := range l.broadcast {
		l.subMu.RLock()
		subscribers := make([]chan string, 0, len(l.subscribers))
		for ch := range l.subscribers {
			subscribers = append(subscribers, ch)
		}
		l.subMu.RUnlock()
		
		// Send to all subscribers (non-blocking)
		for _, ch := range subscribers {
			select {
			case ch <- logLine:
			default:
				// Channel full, skip this subscriber
			}
		}
	}
}

// logMessage writes a log message and broadcasts it
func (l *Logger) logMessage(level, format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return
	}

	message := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	// Write to log
	if l.logger != nil {
		l.logger.Output(3, logLine)
	}

	// Broadcast to subscribers (non-blocking)
	select {
	case l.broadcast <- logLine:
	default:
		// Channel full, skip broadcast to prevent blocking
	}
}

// Printf logs a message at INFO level
func (l *Logger) Printf(format string, v ...interface{}) {
	l.logMessage("INFO", format, v...)
}

// Print logs a message at INFO level
func (l *Logger) Print(v ...interface{}) {
	l.logMessage("INFO", "%s", fmt.Sprint(v...))
}

// Println logs a message at INFO level
func (l *Logger) Println(v ...interface{}) {
	l.logMessage("INFO", "%s", fmt.Sprint(v...))
}

// Errorf logs a message at ERROR level
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logMessage("ERROR", format, v...)
}

// Error logs a message at ERROR level
func (l *Logger) Error(v ...interface{}) {
	l.logMessage("ERROR", "%s", fmt.Sprint(v...))
}

// Warnf logs a message at WARN level
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logMessage("WARN", format, v...)
}

// Warn logs a message at WARN level
func (l *Logger) Warn(v ...interface{}) {
	l.logMessage("WARN", "%s", fmt.Sprint(v...))
}

// Debugf logs a message at DEBUG level
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logMessage("DEBUG", format, v...)
}

// Debug logs a message at DEBUG level
func (l *Logger) Debug(v ...interface{}) {
	l.logMessage("DEBUG", "%s", fmt.Sprint(v...))
}

// Fatal logs a message at FATAL level and exits
func (l *Logger) Fatal(v ...interface{}) {
	l.logMessage("FATAL", "%s", fmt.Sprint(v...))
	os.Exit(1)
}

// Fatalf logs a message at FATAL level and exits
func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.logMessage("FATAL", format, v...)
	os.Exit(1)
}

// Close closes the log file and stops broadcasting
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true
	close(l.broadcast)

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Package-level convenience functions
func Printf(format string, v ...interface{}) {
	GetDefault().Printf(format, v...)
}

func Print(v ...interface{}) {
	GetDefault().Print(v...)
}

func Println(v ...interface{}) {
	GetDefault().Println(v...)
}

func Errorf(format string, v ...interface{}) {
	GetDefault().Errorf(format, v...)
}

func Error(v ...interface{}) {
	GetDefault().Error(v...)
}

func Warnf(format string, v ...interface{}) {
	GetDefault().Warnf(format, v...)
}

func Warn(v ...interface{}) {
	GetDefault().Warn(v...)
}

func Debugf(format string, v ...interface{}) {
	GetDefault().Debugf(format, v...)
}

func Debug(v ...interface{}) {
	GetDefault().Debug(v...)
}

func Fatal(v ...interface{}) {
	GetDefault().Fatal(v...)
}

func Fatalf(format string, v ...interface{}) {
	GetDefault().Fatalf(format, v...)
}

