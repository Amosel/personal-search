package model

import (
	"encoding/json"
	"testing"
)

func TestDocument_Validate_Valid(t *testing.T) {
	doc := Document{
		ID:        "test-id",
		Source:    "chatgpt",
		Type:      "message",
		Timestamp: 1234567890000,
		Text:      "test content",
		Metadata:  map[string]any{"key": "value"},
	}

	if err := doc.Validate(); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
}

func TestDocument_Validate_MissingID(t *testing.T) {
	doc := Document{
		Source:    "chatgpt",
		Type:      "message",
		Timestamp: 1234567890000,
		Text:      "test",
	}

	err := doc.Validate()
	if err != ErrMissingID {
		t.Fatalf("expected ErrMissingID, got %v", err)
	}
}

func TestDocument_Validate_MissingSource(t *testing.T) {
	doc := Document{
		ID:        "test-id",
		Type:      "message",
		Timestamp: 1234567890000,
		Text:      "test",
	}

	err := doc.Validate()
	if err != ErrMissingSource {
		t.Fatalf("expected ErrMissingSource, got %v", err)
	}
}

func TestDocument_Validate_MissingType(t *testing.T) {
	doc := Document{
		ID:        "test-id",
		Source:    "chatgpt",
		Timestamp: 1234567890000,
		Text:      "test",
	}

	err := doc.Validate()
	if err != ErrMissingType {
		t.Fatalf("expected ErrMissingType, got %v", err)
	}
}

func TestDocument_Validate_InvalidTimestamp(t *testing.T) {
	cases := []int64{0, -1, -1000}

	for _, ts := range cases {
		doc := Document{
			ID:        "test-id",
			Source:    "chatgpt",
			Type:      "message",
			Timestamp: ts,
			Text:      "test",
		}

		err := doc.Validate()
		if err != ErrInvalidTimestamp {
			t.Fatalf("timestamp %d: expected ErrInvalidTimestamp, got %v", ts, err)
		}
	}
}

func TestDocument_Validate_EmptyText(t *testing.T) {
	doc := Document{
		ID:        "test-id",
		Source:    "chatgpt",
		Type:      "message",
		Timestamp: 1234567890000,
		Text:      "",
	}

	err := doc.Validate()
	if err != ErrEmptyText {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}
}

func TestDocument_JSONRoundtrip(t *testing.T) {
	original := Document{
		ID:        "test-id",
		Source:    "chatgpt",
		Type:      "message",
		Timestamp: 1234567890000,
		Text:      "test content",
		Metadata:  map[string]any{"author": "user", "thread_id": "conv123"},
		Keywords:  []string{"test", "keyword"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Document
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: %s != %s", decoded.ID, original.ID)
	}
	if decoded.Source != original.Source {
		t.Errorf("Source mismatch")
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch")
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch")
	}
	if decoded.Text != original.Text {
		t.Errorf("Text mismatch")
	}
}
