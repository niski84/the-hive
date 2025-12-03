// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package watcher

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/the-hive/internal/drone/database"
)

// IngestType represents the type of ingestion
type IngestType string

const (
	IngestTypeNew    IngestType = "new"
	IngestTypeUpdate IngestType = "update"
)

// FileDecision represents the decision made about a file
type FileDecision struct {
	FilePath   string
	FileHash   string
	IngestType IngestType
	ShouldProcess bool
	Reason     string
}

// DecisionEngine makes decisions about whether to process files
type DecisionEngine struct {
	db *database.ClientDB
}

// NewDecisionEngine creates a new decision engine
func NewDecisionEngine(db *database.ClientDB) *DecisionEngine {
	return &DecisionEngine{db: db}
}

// Decide determines whether and how to process a file
func (de *DecisionEngine) Decide(filePath string) (*FileDecision, error) {
	decision := &FileDecision{
		FilePath:     filePath,
		ShouldProcess: false,
	}

	// Check file size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() == 0 {
		decision.Reason = "File is empty"
		return decision, nil
	}

	// Calculate file hash
	hash, err := de.calculateFileHash(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}
	decision.FileHash = hash

	// Check database
	trackedFile, err := de.db.GetTrackedFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to query database: %w", err)
	}

	if trackedFile == nil {
		// Case A: New file
		decision.IngestType = IngestTypeNew
		decision.ShouldProcess = true
		decision.Reason = "New file detected"
		log.Printf("New file: %s (hash: %s)", filePath, hash)
	} else if trackedFile.FileHash != hash {
		// Case B: Updated file
		decision.IngestType = IngestTypeUpdate
		decision.ShouldProcess = true
		decision.Reason = fmt.Sprintf("File updated (old hash: %s, new hash: %s)", trackedFile.FileHash, hash)
		log.Printf("File updated: %s", filePath)
	} else {
		// Case C: Stale file
		decision.ShouldProcess = false
		decision.Reason = "File unchanged (hash matches)"
		log.Printf("File unchanged: %s (hash: %s)", filePath, hash)
	}

	return decision, nil
}

// MarkProcessed marks a file as processed in the database
func (de *DecisionEngine) MarkProcessed(decision *FileDecision, serverStatus string) error {
	return de.db.UpsertTrackedFile(decision.FilePath, decision.FileHash, serverStatus)
}

// calculateFileHash calculates SHA-256 hash of file content
func (de *DecisionEngine) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

