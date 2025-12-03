// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

var (
	outputDir = flag.String("output", "./test_watch_dir", "Output directory for test files")
)

func main() {
	flag.Parse()

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	fmt.Printf("🌱 Seeding test data to: %s\n", *outputDir)

	// Generate 5 Markdown files with unique searchable phrases
	markdownFiles := []struct {
		filename string
		content  string
		phrase   string
	}{
		{
			filename: "project_alpha.md",
			phrase:   "Project Alpha confidential report",
			content: `# Project Alpha Confidential Report

## Executive Summary

This document contains confidential information about Project Alpha. The project involves advanced research and development in artificial intelligence and machine learning systems.

## Key Findings

- Project Alpha has made significant progress in neural network optimization
- The team has developed new algorithms for efficient training
- Performance metrics show 40% improvement over baseline systems

## Recommendations

We recommend continuing investment in Project Alpha as it shows great promise for future applications.
`,
		},
		{
			filename: "beta_analysis.md",
			phrase:   "Beta analysis quarterly results",
			content: `# Beta Analysis - Q4 Results

## Overview

This quarterly analysis covers the performance of Beta systems during the fourth quarter. Results show strong growth and improved efficiency.

## Financial Metrics

- Revenue increased by 25% compared to Q3
- Operating costs decreased by 10%
- Net profit margin improved to 18%

## Technical Achievements

The Beta team successfully deployed new infrastructure that reduced latency by 30%.
`,
		},
		{
			filename: "gamma_protocol.md",
			phrase:   "Gamma protocol implementation guide",
			content: `# Gamma Protocol Implementation Guide

## Introduction

The Gamma Protocol is a new communication standard designed for high-performance distributed systems. This guide provides detailed implementation instructions.

## Protocol Specification

The protocol uses a binary format with the following structure:
- Header: 16 bytes
- Payload: Variable length
- Checksum: 4 bytes

## Implementation Steps

1. Initialize the protocol handler
2. Configure connection parameters
3. Establish secure channel
4. Begin data transmission

## Security Considerations

All communications must be encrypted using AES-256. Authentication is required before any data exchange.
`,
		},
		{
			filename: "delta_research.md",
			phrase:   "Delta research findings summary",
			content: `# Delta Research Findings Summary

## Research Objectives

The Delta research project aimed to investigate novel approaches to data compression and storage optimization.

## Methodology

We conducted experiments using various compression algorithms including:
- LZ77 variants
- Huffman encoding
- Arithmetic coding
- Dictionary-based methods

## Results

Our findings indicate that a hybrid approach combining dictionary-based compression with arithmetic coding yields the best results, achieving 60% compression ratio on average.

## Conclusion

The Delta research has successfully identified optimal compression strategies for our use case.
`,
		},
		{
			filename: "epsilon_design.md",
			phrase:   "Epsilon design document architecture",
			content: `# Epsilon Design Document

## Architecture Overview

The Epsilon system is designed as a microservices architecture with the following components:
- API Gateway
- Authentication Service
- Data Processing Service
- Storage Service
- Notification Service

## Component Details

### API Gateway
Handles all incoming requests and routes them to appropriate services.

### Authentication Service
Manages user authentication and authorization using JWT tokens.

### Data Processing Service
Performs complex data transformations and calculations.

## Deployment

The system is deployed using Kubernetes with auto-scaling enabled. Each service runs in its own pod with resource limits configured.
`,
		},
	}

	createdFiles := []string{}

	// Write markdown files
	for _, md := range markdownFiles {
		filePath := filepath.Join(*outputDir, md.filename)
		if err := os.WriteFile(filePath, []byte(md.content), 0644); err != nil {
			log.Printf("Failed to write %s: %v", md.filename, err)
			continue
		}
		createdFiles = append(createdFiles, md.filename)
		fmt.Printf("✅ Created: %s (phrase: '%s')\n", md.filename, md.phrase)
	}

	// Download sample PDF
	pdfURL := "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf"
	pdfPath := filepath.Join(*outputDir, "sample.pdf")

	fmt.Printf("📥 Downloading sample PDF from %s...\n", pdfURL)

	resp, err := http.Get(pdfURL)
	if err != nil {
		log.Printf("Warning: Failed to download PDF: %v", err)
		log.Printf("You can manually add a PDF file to %s for testing", *outputDir)
	} else {
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Warning: PDF download returned status %d", resp.StatusCode)
		} else {
			out, err := os.Create(pdfPath)
			if err != nil {
				log.Printf("Warning: Failed to create PDF file: %v", err)
			} else {
				defer out.Close()

				_, err = io.Copy(out, resp.Body)
				if err != nil {
					log.Printf("Warning: Failed to write PDF: %v", err)
				} else {
					createdFiles = append(createdFiles, "sample.pdf")
					fmt.Printf("✅ Downloaded: sample.pdf\n")
				}
			}
		}
	}

	// Summary
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   Created %d files in %s\n", len(createdFiles), *outputDir)
	fmt.Printf("   Files:\n")
	for _, file := range createdFiles {
		fmt.Printf("     - %s\n", file)
	}
	fmt.Printf("\n✨ Seeding complete! You can now test the file watcher with these files.\n")
}
