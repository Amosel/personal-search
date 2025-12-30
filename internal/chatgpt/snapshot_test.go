package chatgpt

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// TestSnapshot_RealExport validates ingestion against a real ChatGPT export.
// This test is skipped if the CHATGPT_EXPORT_PATH environment variable is not set.
func TestSnapshot_RealExport(t *testing.T) {
	exportPath := os.Getenv("CHATGPT_EXPORT_PATH")
	if exportPath == "" {
		t.Skip("CHATGPT_EXPORT_PATH not set, skipping snapshot test")
	}

	// Load the export
	exp, err := LoadExport(exportPath)
	if err != nil {
		t.Fatalf("Failed to load export: %v", err)
	}

	// Convert to documents
	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("Failed to convert to documents: %v", err)
	}

	// Expected values from real export
	// Update these when export format changes
	const (
		expectedConversations = 1475
		expectedDocuments     = 18474
		expectedHash          = "d5322656" // First 8 chars of determinism hash
	)

	// Validate conversation count
	if len(exp.Conversations) != expectedConversations {
		t.Errorf("conversation count mismatch: got %d, want %d", len(exp.Conversations), expectedConversations)
	}

	// Validate document count
	if len(docs) != expectedDocuments {
		t.Errorf("document count mismatch: got %d, want %d", len(docs), expectedDocuments)
	}

	// Validate determinism hash
	h := sha256.New()
	for _, doc := range docs {
		h.Write([]byte(doc.ID))
	}
	hash := hex.EncodeToString(h.Sum(nil))[:8]

	if hash != expectedHash {
		t.Errorf("determinism hash mismatch: got %s, want %s", hash, expectedHash)
	}

	// Validate sample documents
	if len(docs) > 0 {
		first := docs[0]
		if first.Source != "chatgpt" {
			t.Errorf("first doc source: got %s, want chatgpt", first.Source)
		}
		if first.Type != "message" {
			t.Errorf("first doc type: got %s, want message", first.Type)
		}
		if first.Timestamp <= 0 {
			t.Errorf("first doc timestamp: got %d, want > 0", first.Timestamp)
		}
		if first.Text == "" {
			t.Error("first doc text is empty")
		}
		if first.Metadata["author"] == "" {
			t.Error("first doc missing author metadata")
		}
		if first.Metadata["thread_id"] == "" {
			t.Error("first doc missing thread_id metadata")
		}
	}

	// Validate all documents have required fields
	for i, doc := range docs {
		if err := doc.Validate(); err != nil {
			t.Errorf("document %d failed validation: %v", i, err)
			break // Only report first failure
		}
	}
}
