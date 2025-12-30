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

	// Count by timestamp and role
	totalMsgs := 0
	validTimestamp := 0
	invalidTimestamp := 0
	roleCount := make(map[string]int)

	for _, conv := range exp.Conversations {
		for _, msg := range conv.Mapping {
			if msg.Message == nil {
				continue
			}
			if msg.Message.Author.Role == "system" {
				continue
			}

			totalMsgs++
			roleCount[msg.Message.Author.Role]++

			if msg.CreateTime > 0 {
				validTimestamp++
			} else {
				invalidTimestamp++
			}
		}
	}

	fmt.Printf("Total messages (after nil/system filter): %d\n", totalMsgs)
	fmt.Printf("Valid timestamps (>0): %d\n", validTimestamp)
	fmt.Printf("Invalid timestamps (≤0): %d\n", invalidTimestamp)
	fmt.Printf("\nRole distribution:\n")
	for role, count := range roleCount {
		fmt.Printf("  %s: %d\n", role, count)
	}
}
