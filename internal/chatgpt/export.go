package chatgpt

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Export represents the ChatGPT export structure.
type Export struct {
	Conversations []Conversation `json:"conversations"`
}

// Conversation represents a single conversation thread.
type Conversation struct {
	ID      string             `json:"id"`
	Title   string             `json:"title,omitempty"`
	Mapping map[string]Message `json:"mapping,omitempty"`
}

// Message represents a single message node in the conversation mapping.
// This is the outer wrapper in the mapping structure.
type Message struct {
	ID      string   `json:"id"`
	Message *MsgBody `json:"message,omitempty"`
	Parent  string   `json:"parent,omitempty"`
}

// MsgBody contains the actual message content.
// In real ChatGPT exports, create_time lives HERE at the message body level.
type MsgBody struct {
	ID         string         `json:"id,omitempty"`
	Author     Author         `json:"author"`
	Content    Content        `json:"content"`
	CreateTime float64        `json:"create_time,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Author identifies who sent the message.
type Author struct {
	Role string `json:"role"` // "user" | "assistant" | "system"
}

// Content contains the message text.
type Content struct {
	ContentType string `json:"content_type"` // "text"
	Parts       []any  `json:"parts"`        // often []string but can vary
	Text        string `json:"text,omitempty"`
}

// LoadExport loads and parses a ChatGPT export.
// Accepts either a JSON file or ZIP archive containing conversations.json.
// Returns error if file doesn't exist or JSON is malformed.
// Handles both {conversations:[]} and [] array formats.
func LoadExport(path string) (*Export, error) {
	// Check if path is a ZIP file
	if strings.HasSuffix(strings.ToLower(path), ".zip") {
		return loadFromZIP(path)
	}

	// Load as JSON file
	return loadFromJSON(path)
}

// loadFromZIP extracts and parses conversations.json from a ZIP archive.
func loadFromZIP(zipPath string) (*Export, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP: %w", err)
	}
	defer r.Close()

	// Find conversations.json
	for _, f := range r.File {
		if filepath.Base(f.Name) == "conversations.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open conversations.json: %w", err)
			}
			defer rc.Close()

			return parseJSON(rc)
		}
	}

	return nil, fmt.Errorf("conversations.json not found in ZIP archive")
}

// loadFromJSON loads and parses a JSON file.
func loadFromJSON(path string) (*Export, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseJSON(f)
}

// parseJSON decodes JSON from a reader using streaming.
// Handles both {conversations:[]} and [] array formats.
func parseJSON(r io.Reader) (*Export, error) {
	dec := json.NewDecoder(r)

	// Peek at first token to determine format
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}

	// Array format: [...]
	if delim, ok := t.(json.Delim); ok && delim == '[' {
		convs := make([]Conversation, 0)
		for dec.More() {
			var conv Conversation
			if err := dec.Decode(&conv); err != nil {
				return nil, err
			}
			convs = append(convs, conv)
		}
		// Consume closing ]
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return &Export{Conversations: convs}, nil
	}

	// Object format: {conversations: [...]}
	if delim, ok := t.(json.Delim); ok && delim == '{' {
		var convs []Conversation
		foundConversations := false
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := tok.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key token %v", tok)
			}
			if key != "conversations" {
				var discard json.RawMessage
				if err := dec.Decode(&discard); err != nil {
					return nil, err
				}
				continue
			}
			if err := dec.Decode(&convs); err != nil {
				return nil, err
			}
			foundConversations = true
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		if foundConversations {
			return &Export{Conversations: convs}, nil
		}
	}

	return nil, fmt.Errorf("unrecognized export format")
}
