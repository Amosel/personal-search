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

	// Load export
	exp, err := chatgpt.LoadExport(path)
	if err != nil {
		log.Fatalf("Failed to load export: %v", err)
	}

	fmt.Printf("Loaded %d conversations\n", len(exp.Conversations))

	// Sample first conversation
	if len(exp.Conversations) > 0 {
		conv := exp.Conversations[0]
		fmt.Printf("\nFirst conversation:\n")
		fmt.Printf("  ID: %s\n", conv.ID)
		fmt.Printf("  Title: %s\n", conv.Title)
		fmt.Printf("  Messages in mapping: %d\n", len(conv.Mapping))

		// Sample first message
		for msgID, msg := range conv.Mapping {
			fmt.Printf("\n  First message:\n")
			fmt.Printf("    ID: %s\n", msgID)
			fmt.Printf("    CreateTime: %f\n", msg.CreateTime)
			if msg.Message != nil {
				fmt.Printf("    Role: %s\n", msg.Message.Author.Role)
				fmt.Printf("    Content type: %s\n", msg.Message.Content.ContentType)
				fmt.Printf("    Parts count: %d\n", len(msg.Message.Content.Parts))
				if len(msg.Message.Content.Parts) > 0 {
					fmt.Printf("    First part type: %T\n", msg.Message.Content.Parts[0])
					fmt.Printf("    First part value: %v\n", msg.Message.Content.Parts[0])
				}
			} else {
				fmt.Printf("    Message body is nil\n")
			}
			break
		}
	}

	// Try converting
	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		log.Fatalf("Failed to convert: %v", err)
	}

	fmt.Printf("\nDocuments created: %d\n", len(docs))
}
