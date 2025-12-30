package chatgpt

import (
	"fmt"
	"sort"
	"time"

	"personal-search/internal/model"
)

// ToDocuments converts a ChatGPT export to canonical Documents.
// This is the SOLE enforcement point for Document.Validate().
// Returns error if any document fails validation.
// Skips system messages and messages with empty canonicalized text.
func ToDocuments(e *Export) ([]model.Document, error) {
	var docs []model.Document

	for _, conv := range e.Conversations {
		// Gather messages from mapping
		var msgs []Message
		for _, m := range conv.Mapping {
			// Skip if no message body
			if m.Message == nil {
				continue
			}

			// Skip system messages
			if m.Message.Author.Role == "system" {
				continue
			}

			msgs = append(msgs, m)
		}

		// Sort by create_time for deterministic ordering
		sort.Slice(msgs, func(i, j int) bool {
			if msgs[i].CreateTime == msgs[j].CreateTime {
				// Stable sort for identical timestamps
				return msgs[i].ID < msgs[j].ID
			}
			return msgs[i].CreateTime < msgs[j].CreateTime
		})

		// Convert each message to a Document
		for _, m := range msgs {
			// Canonicalize text
			text := extractAndCanonicalizeText(m.Message)

			// DEFECT: EMPTY_TEXT_AFTER_CANON → skip
			if text == "" {
				continue
			}

			// Convert timestamp to unix milliseconds
			timestampMs := int64(m.CreateTime * 1000)

			// DEFECT: INVALID_TIMESTAMP → skip
			if timestampMs <= 0 {
				continue
			}

			// Build metadata
			metadata := map[string]any{
				"author":    m.Message.Author.Role,
				"thread_id": conv.ID,
			}
			if conv.Title != "" {
				metadata["title"] = conv.Title
			}
			if timestampMs > 0 {
				metadata["created"] = time.UnixMilli(timestampMs).UTC().Format(time.RFC3339)
			}

			// Construct document
			doc := model.Document{
				ID:        model.StableID(conv.ID, m.ID),
				Source:    "chatgpt",
				Type:      "message",
				Timestamp: timestampMs,
				Text:      text,
				Metadata:  metadata,
			}

			// SOLE ENFORCEMENT POINT: Validate before adding
			if err := doc.Validate(); err != nil {
				return nil, fmt.Errorf("document validation failed for conv=%s msg=%s: %w", conv.ID, m.ID, err)
			}

			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// extractAndCanonicalizeText extracts text from message content and canonicalizes it.
func extractAndCanonicalizeText(m *MsgBody) string {
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
		return CanonicalizeText(text)
	}

	// Fallback to content.text
	if m.Content.Text != "" {
		return CanonicalizeText(m.Content.Text)
	}

	return ""
}
