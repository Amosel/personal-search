package main

import (
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

	exp, err := chatgpt.LoadExport(path)
	if err != nil {
		log.Fatalf("Failed to load export: %v", err)
	}

	// Track skip reasons
	totalRaw := 0
	skippedNilMsg := 0
	skippedSystem := 0
	skippedEmptyText := 0
	skippedInvalidTimestamp := 0
	wouldBeValid := 0

	for _, conv := range exp.Conversations {
		for _, msg := range conv.Mapping {
			totalRaw++

			// Skip if no message body
			if msg.Message == nil {
				skippedNilMsg++
				continue
			}

			// Skip system messages
			if msg.Message.Author.Role == "system" {
				skippedSystem++
				continue
			}

			// Check text
			text := extractAndCanonicalizeText(msg.Message)
			if text == "" {
				skippedEmptyText++
				continue
			}

			// Check timestamp
			timestampMs := int64(msg.CreateTime * 1000)
			if timestampMs <= 0 {
				skippedInvalidTimestamp++
				continue
			}

			wouldBeValid++
		}
	}

	fmt.Printf("Total messages in mapping: %d\n", totalRaw)
	fmt.Printf("Skipped - nil message: %d\n", skippedNilMsg)
	fmt.Printf("Skipped - system role: %d\n", skippedSystem)
	fmt.Printf("Skipped - empty text: %d\n", skippedEmptyText)
	fmt.Printf("Skipped - invalid timestamp: %d\n", skippedInvalidTimestamp)
	fmt.Printf("Would create documents: %d\n", wouldBeValid)
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
