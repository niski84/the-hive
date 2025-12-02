package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/the-hive/internal/client"
	"github.com/the-hive/internal/pdf"
	"github.com/the-hive/internal/proto"
)

// End of import block

var (
	watchDir     = flag.String("watch-dir", "./watch", "Directory to watch for PDF files")
	hiveAddr     = flag.String("hive-addr", "localhost:50051", "Hive server address")
	pollInterval = flag.Duration("poll-interval", 5*time.Second, "Polling interval for file changes")
)

func main() {
	flag.Parse()

	// Validate watch directory
	if _, err := os.Stat(*watchDir); os.IsNotExist(err) {
		log.Printf("Watch directory does not exist, creating: %s", *watchDir)
		if err := os.MkdirAll(*watchDir, 0755); err != nil {
			log.Fatalf("Failed to create watch directory: %v", err)
		}
	}

	// Connect to Hive server
	conn, err := grpc.Dial(*hiveAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Hive server: %v", err)
	}
	defer conn.Close()

	hiveClient := proto.NewHiveClient(conn)
	droneClient := client.NewDroneClient(hiveClient)

	// Initialize PDF processor
	pdfProcessor := pdf.NewProcessor()

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// Watch the directory
	err = watcher.Add(*watchDir)
	if err != nil {
		log.Fatalf("Failed to watch directory: %v", err)
	}

	log.Printf("Drone client started. Watching directory: %s", *watchDir)
	log.Printf("Connected to Hive server at: %s", *hiveAddr)

	// Process existing PDFs in the directory
	go processExistingFiles(*watchDir, droneClient, pdfProcessor)

	// Watch for new files
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if filepath.Ext(event.Name) == ".pdf" {
					log.Printf("Detected PDF file: %s", event.Name)
					go processPDFFile(event.Name, droneClient, pdfProcessor)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func processExistingFiles(dir string, droneClient *client.DroneClient, pdfProcessor *pdf.Processor) {
	log.Printf("Scanning existing PDFs in %s", dir)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".pdf" {
			processPDFFile(path, droneClient, pdfProcessor)
		}
		return nil
	})

	if err != nil {
		log.Printf("Error scanning directory: %v", err)
	}
}

func processPDFFile(filePath string, droneClient *client.DroneClient, pdfProcessor *pdf.Processor) {
	log.Printf("Processing PDF: %s", filePath)

	// Extract text from PDF
	text, err := pdfProcessor.ExtractText(filePath)
	if err != nil {
		log.Printf("Failed to extract text from %s: %v", filePath, err)
		return
	}

	if text == "" {
		log.Printf("No text extracted from %s", filePath)
		return
	}

	// Chunk the text
	chunks, err := pdfProcessor.ChunkText(text)
	if err != nil {
		log.Printf("Failed to chunk text from %s: %v", filePath, err)
		return
	}

	log.Printf("Extracted %d chunks from %s", len(chunks), filePath)

	// Send chunks to Hive
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	documentID := filepath.Base(filePath)
	for i, chunk := range chunks {
		err := droneClient.IngestChunk(ctx, documentID, chunk, i, map[string]string{
			"filename": filepath.Base(filePath),
			"path":     filePath,
		})
		if err != nil {
			log.Printf("Failed to ingest chunk %d from %s: %v", i, filePath, err)
			continue
		}
	}

	log.Printf("Successfully processed %s", filePath)
}
