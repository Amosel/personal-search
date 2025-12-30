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

	fmt.Printf("=== ChatGPT Export Analysis ===\n\n")

	// 1. High-level stats
	fmt.Printf("Export Overview:\n")
	fmt.Printf("  Conversations: %d\n", len(exp.Conversations))

	totalRaw := 0
	for _, conv := range exp.Conversations {
		totalRaw += len(conv.Mapping)
	}
	fmt.Printf("  Total message nodes: %d\n", totalRaw)

	// 2. Sample first conversation
	if len(exp.Conversations) > 0 {
		conv := exp.Conversations[0]
		fmt.Printf("\nFirst Conversation Sample:\n")
		fmt.Printf("  ID: %s\n", conv.ID)
		fmt.Printf("  Title: %s\n", conv.Title)
		fmt.Printf("  Message nodes: %d\n", len(conv.Mapping))

		// Sample first non-nil message
		for msgID, msg := range conv.Mapping {
			if msg.Message != nil {
				fmt.Printf("\n  First Message:\n")
				fmt.Printf("    ID: %s\n", msgID)
				fmt.Printf("    Role: %s\n", msg.Message.Author.Role)
				fmt.Printf("    CreateTime: %f\n", msg.Message.CreateTime)
				fmt.Printf("    Content type: %s\n", msg.Message.Content.ContentType)
				fmt.Printf("    Parts count: %d\n", len(msg.Message.Content.Parts))
				if len(msg.Message.Content.Parts) > 0 {
					fmt.Printf("    First part type: %T\n", msg.Message.Content.Parts[0])
				}
				break
			}
		}
	}

	// 3. Defect analysis - track skip reasons
	skippedNilMsg := 0
	skippedSystem := 0
	skippedEmptyText := 0
	skippedInvalidTimestamp := 0
	roleCount := make(map[string]int)
	validTimestamp := 0
	invalidTimestamp := 0

	for _, conv := range exp.Conversations {
		for _, msg := range conv.Mapping {
			// Skip if no message body
			if msg.Message == nil {
				skippedNilMsg++
				continue
			}

			role := msg.Message.Author.Role

			// Skip system messages
			if role == "system" {
				skippedSystem++
				continue
			}

			roleCount[role]++

			// Check timestamp
			if msg.Message.CreateTime > 0 {
				validTimestamp++
			} else {
				invalidTimestamp++
			}

			// Check text
			text := extractAndCanonicalizeText(msg.Message)
			if text == "" {
				skippedEmptyText++
				continue
			}

			// Check timestamp for document creation
			timestampMs := int64(msg.Message.CreateTime * 1000)
			if timestampMs <= 0 {
				skippedInvalidTimestamp++
				continue
			}
		}
	}

	fmt.Printf("\nDefect Analysis:\n")
	fmt.Printf("  Nil message body: %d\n", skippedNilMsg)
	fmt.Printf("  System role: %d\n", skippedSystem)
	fmt.Printf("  Empty text (after canon): %d\n", skippedEmptyText)
	fmt.Printf("  Invalid timestamp: %d\n", skippedInvalidTimestamp)

	totalNonSystem := 0
	for _, count := range roleCount {
		totalNonSystem += count
	}

	fmt.Printf("\nTimestamp Distribution (non-system messages):\n")
	fmt.Printf("  Valid (>0): %d (%.1f%%)\n", validTimestamp, float64(validTimestamp)/float64(totalNonSystem)*100)
	fmt.Printf("  Invalid (≤0): %d (%.1f%%)\n", invalidTimestamp, float64(invalidTimestamp)/float64(totalNonSystem)*100)

	fmt.Printf("\nRole Distribution (non-system):\n")
	for role, count := range roleCount {
		fmt.Printf("  %s: %d\n", role, count)
	}

	// 4. Document conversion
	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		log.Fatalf("Failed to convert to documents: %v", err)
	}

	fmt.Printf("\nDocument Conversion:\n")
	fmt.Printf("  Documents created: %d\n", len(docs))
	reduction := float64(totalRaw-len(docs)) / float64(totalRaw) * 100
	fmt.Printf("  Reduction: %.1f%%\n", reduction)

	// 5. Determinism hash
	h := sha256.New()
	for _, doc := range docs {
		h.Write([]byte(doc.ID))
	}
	hash := hex.EncodeToString(h.Sum(nil))[:8]
	fmt.Printf("  Determinism hash: %s\n", hash)

	// 6. Sample documents
	if len(docs) > 0 {
		fmt.Printf("\nFirst Document Sample:\n")
		doc := docs[0]
		fmt.Printf("  ID: %s\n", doc.ID)
		fmt.Printf("  Source: %s\n", doc.Source)
		fmt.Printf("  Type: %s\n", doc.Type)
		fmt.Printf("  Timestamp: %d\n", doc.Timestamp)
		fmt.Printf("  Author: %v\n", doc.Metadata["author"])
		fmt.Printf("  Thread ID: %v\n", doc.Metadata["thread_id"])
		fmt.Printf("  Text (first 100 chars): %s...\n", truncate(doc.Text, 100))
	}
}

func extractAndCanonicalizeText(m *chatgpt.MsgBody) string {
	if m == nil {
		return ""
	}

	// Try content.parts first
	if len(m.Content.Parts) > 0 {
		var text string
		for _, part := range m.Content.Parts {
			switch v := part.(type) {
			case string:
				text += v + "\n"
			default:
				text += fmt.Sprint(v) + "\n"
			}
		}
		return chatgpt.CanonicalizeText(text)
	}

	// Fallback to content.text
	if m.Content.Text != "" {
		return chatgpt.CanonicalizeText(m.Content.Text)
	}

	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
