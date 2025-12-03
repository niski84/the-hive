// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	logFile    = "hive-server.log"
	timeout    = 30 * time.Second
	failureMsg = "Unable to parse UUID"
	rpcError   = "rpc error"
	successMsg = "vector upsert success"
)

func main() {
	// Create timeout channel
	timeoutChan := time.After(timeout)
	
	// Channel for new lines
	lineChan := make(chan string, 100)
	
	// Start reading new lines (tail-like behavior)
	go func() {
		var file *os.File
		var err error
		
		// Open log file
		for {
			file, err = os.Open(logFile)
			if err != nil {
				// File might not exist yet, wait and retry
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}
		defer file.Close()

		// Seek to end of file to tail
		fileInfo, err := file.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stat log file: %v\n", err)
			return
		}
		file.Seek(fileInfo.Size(), 0)

		for {
			// Check if file still exists and is readable
			fileInfo, err := file.Stat()
			if err != nil {
				// File might have been deleted or rotated, wait and retry
				file.Close()
				time.Sleep(500 * time.Millisecond)
				newFile, err := os.Open(logFile)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				file = newFile
				fileInfo, _ = file.Stat()
				file.Seek(fileInfo.Size(), 0)
				continue
			}
			
			// Get current position
			currentPos, _ := file.Seek(0, 1) // Get current position
			fileSize := fileInfo.Size()
			
			// If file grew, read new lines
			if fileSize > currentPos {
				file.Seek(currentPos, 0)
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					lineChan <- scanner.Text()
				}
			}
			
			// Sleep before checking again
			time.Sleep(200 * time.Millisecond)
		}
	}()

	fmt.Printf("🔍 Watchdog monitoring %s for UUID errors...\n", logFile)
	fmt.Printf("⏱️  Timeout: %v\n", timeout)

	// Monitor for errors or success
	for {
		select {
		case line := <-lineChan:
			lineLower := strings.ToLower(line)
			
			// Check for failure conditions
			if strings.Contains(lineLower, strings.ToLower(failureMsg)) || 
			   strings.Contains(lineLower, strings.ToLower(rpcError)) {
				fmt.Printf("❌ FAILURE DETECTED: %s\n", line)
				os.Exit(1)
			}
			
			// Check for success condition
			if strings.Contains(lineLower, strings.ToLower(successMsg)) {
				fmt.Printf("✅ SUCCESS DETECTED: %s\n", line)
				os.Exit(0)
			}
			
		case <-timeoutChan:
			fmt.Printf("⏰ Timeout after %v - no success or failure detected\n", timeout)
			os.Exit(1)
		}
	}
}

