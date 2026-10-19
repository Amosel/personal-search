package chatgpt

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"personal-search/internal/model"
)

// ToDocuments converts a ChatGPT export to canonical Documents.
// This is the SOLE enforcement point for Document.Validate().
// Returns error if any document fails validation.
// Skips system messages and messages with empty canonicalized text.
func ToDocuments(e *Export) ([]model.Document, error) {
	return ToDocumentsWithReport(e, nil)
}

func ToDocumentsWithReport(e *Export, report *IngestReport) ([]model.Document, error) {
	var docs []model.Document

	for _, conv := range e.Conversations {
		// Gather messages from mapping
		var msgs []Message
		for _, m := range conv.Mapping {
			msgs = append(msgs, m)
		}

		// Sort by create_time for deterministic ordering
		sort.Slice(msgs, func(i, j int) bool {
			iTime := messageCreateTime(msgs[i])
			jTime := messageCreateTime(msgs[j])
			if iTime == jTime {
				// Stable sort for identical timestamps
				return msgs[i].ID < msgs[j].ID
			}
			return iTime < jTime
		})

		// Convert each message to a Document
		for _, m := range msgs {
			classified := classifyMessage(conv, m)
			report.Append(classified.Entry)
			if classified.Entry.Outcome == OutcomeSkip {
				continue
			}
			if classified.Entry.Outcome == OutcomeFail {
				return nil, fmt.Errorf("classification failed for conv=%s msg=%s: %s", conv.ID, m.ID, classified.Entry.Error)
			}

			// Construct document
			doc := model.Document{
				ID:        classified.Entry.DocumentID,
				Source:    "chatgpt",
				Type:      "message",
				Timestamp: classified.TimestampMS,
				Text:      classified.Text,
				Metadata:  classified.Metadata,
			}

			doc.SetExtractedText(classified.OriginalText, classified.Text, "chatgpt-text-v1")

			// SOLE ENFORCEMENT POINT: Validate before adding
			if err := doc.Validate(); err != nil {
				failEntry := ClassificationEntry{
					RecordID: SourceRecordID{
						Source:         "chatgpt",
						ConversationID: conv.ID,
						MessageID:      m.ID,
					},
					Outcome:         OutcomeFail,
					Reason:          ReasonFailDocumentValidation,
					Author:          classified.Entry.Author,
					TimestampUnixMS: classified.TimestampMS,
					DocumentID:      classified.Entry.DocumentID,
					TextLen:         len(classified.Text),
					Error:           err.Error(),
				}
				report.Append(failEntry)
				return nil, fmt.Errorf("document validation failed for conv=%s msg=%s: %w", conv.ID, m.ID, err)
			}

			docs = append(docs, doc)
		}
	}

	return docs, nil
}

func messageCreateTime(m Message) float64 {
	if m.Message == nil {
		return 0
	}
	return m.Message.CreateTime
}

type classifiedMessage struct {
	Entry        ClassificationEntry
	Text         string
	OriginalText string
	TimestampMS  int64
	Metadata     map[string]any
}

func classifyMessage(conv Conversation, m Message) classifiedMessage {
	entry := ClassificationEntry{
		RecordID: SourceRecordID{
			Source:         "chatgpt",
			ConversationID: conv.ID,
			MessageID:      m.ID,
		},
	}
	if m.Message == nil {
		entry.Outcome = OutcomeSkip
		entry.Reason = ReasonSkipNilMessage
		return classifiedMessage{Entry: entry}
	}
	entry.Author = m.Message.Author.Role
	timestampMS := int64(m.Message.CreateTime * 1000)
	if timestampMS > 0 {
		entry.TimestampUnixMS = timestampMS
	}
	if m.Message.Author.Role == "system" {
		entry.Outcome = OutcomeSkip
		entry.Reason = ReasonSkipSystemMessage
		return classifiedMessage{Entry: entry}
	}

	original, boundaries, omitted := extractOriginalText(m.Message)
	text := CanonicalizeText(original)
	entry.TextLen = len(text)
	if text == "" {
		entry.Outcome = OutcomeSkip
		entry.Reason = ReasonSkipEmptyTextAfterCanon
		if omitted > 0 && len(boundaries) == 0 {
			entry.Reason = ReasonSkipUnsupportedContent
		}
		return classifiedMessage{Entry: entry}
	}
	if timestampMS <= 0 {
		entry.Outcome = OutcomeSkip
		entry.Reason = ReasonSkipInvalidTimestamp
		return classifiedMessage{Entry: entry}
	}

	metadata := map[string]any{
		"author":                 m.Message.Author.Role,
		"source_node_id":         m.ID,
		"source_message_id":      m.Message.ID,
		"source_parts":           m.Message.Content.Parts,
		"source_part_boundaries": boundaries,
		"non_text_parts":         omitted,
		"text_join_policy":       "newline_between_string_parts",
		"thread_id":              conv.ID,
	}
	if conv.Title != "" {
		metadata["title"] = conv.Title
	}
	metadata["created"] = time.UnixMilli(timestampMS).UTC().Format(time.RFC3339)

	entry.Outcome = OutcomeDocument
	entry.Reason = ReasonDocumentCreated
	entry.DocumentID = model.StableID(conv.ID, m.ID)
	return classifiedMessage{
		Entry:        entry,
		Text:         text,
		OriginalText: original,
		TimestampMS:  timestampMS,
		Metadata:     metadata,
	}
}

// PartBoundary is a byte range in extracted original text; synthetic joins are
// deliberately outside these ranges. Raw parts retain non-text provenance.
type PartBoundary struct {
	Part  int `json:"part"`
	Start int `json:"start"`
	End   int `json:"end"`
}

func extractOriginalText(m *MsgBody) (string, []PartBoundary, int) {
	if m == nil {
		return "", nil, 0
	}
	if len(m.Content.Parts) == 0 {
		return m.Content.Text, nil, 0
	}
	var b strings.Builder
	var boundaries []PartBoundary
	omitted := 0
	for i, part := range m.Content.Parts {
		text, ok := part.(string)
		if !ok {
			omitted++
			continue
		}
		if len(boundaries) > 0 {
			b.WriteByte('\n')
		}
		start := b.Len()
		b.WriteString(text)
		boundaries = append(boundaries, PartBoundary{Part: i, Start: start, End: b.Len()})
	}
	return b.String(), boundaries, omitted
}
func extractAndCanonicalizeText(m *MsgBody) string {
	original, _, _ := extractOriginalText(m)
	return CanonicalizeText(original)
}
