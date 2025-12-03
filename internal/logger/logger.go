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
	file     *os.File
	logger   *log.Logger
	broadcast chan string
	mu       sync.RWMutex
	closed   bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger
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
		file:      file,
		logger:    log.New(multiWriter, "", log.LstdFlags|log.Lshortfile),
		broadcast: make(chan string, 100), // Buffered channel to prevent blocking
		closed:    false,
	}

	return logger, nil
}

// GetDefault returns the default logger instance
func GetDefault() *Logger {
	if defaultLogger == nil {
		// Fallback to stdout-only logger if not initialized
		defaultLogger = &Logger{
			logger:    log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
			broadcast: make(chan string, 100),
		}
	}
	return defaultLogger
}

// Subscribe returns a channel that receives log messages
func (l *Logger) Subscribe() <-chan string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.broadcast
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

