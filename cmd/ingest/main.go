package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"personal-search/internal/chatgpt"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <path-to-export>")
	}

	path := os.Args[1]

	// Load export
	exp, err := chatgpt.LoadExport(path)
	if err != nil {
		log.Fatalf("Failed to load export: %v", err)
	}

	// Convert to documents
	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		log.Fatalf("Failed to convert to documents: %v", err)
	}

	// Compute determinism hash
	h := sha256.New()
	for _, doc := range docs {
		h.Write([]byte(doc.ID))
	}
	hash := hex.EncodeToString(h.Sum(nil))[:8]

	// Report counts
	fmt.Printf("Conversations: %d\n", len(exp.Conversations))

	totalMessages := 0
	for _, conv := range exp.Conversations {
		totalMessages += len(conv.Mapping)
	}
	fmt.Printf("Total messages: %d\n", totalMessages)
	fmt.Printf("Documents created: %d\n", len(docs))
	fmt.Printf("Determinism hash: %s\n", hash)

	if totalMessages > 0 {
		reduction := float64(totalMessages-len(docs)) / float64(totalMessages) * 100
		fmt.Printf("Reduction: %.1f%%\n", reduction)
	}
}
