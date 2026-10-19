package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

var (
	ErrMissingID        = errors.New("document ID is required")
	ErrMissingSource    = errors.New("document source is required")
	ErrMissingType      = errors.New("document type is required")
	ErrInvalidTimestamp = errors.New("document timestamp must be > 0")
	ErrEmptyText        = errors.New("document text is required")
)

// Document is the canonical unit of search.
type Document struct {
	ID                string         `json:"id"`
	Source            string         `json:"source"`
	Type              string         `json:"type"`
	Timestamp         int64          `json:"timestamp_unix_ms"`
	Text              string         `json:"text"`
	OriginalText      *string        `json:"original_text,omitempty"`
	SearchText        string         `json:"search_text,omitempty"`
	TextFidelity      string         `json:"text_fidelity,omitempty"`
	ExtractionVersion string         `json:"extraction_version,omitempty"`
	TextSHA256        string         `json:"text_sha256,omitempty"`
	Metadata          map[string]any `json:"metadata"`
	Keywords          []string       `json:"keywords,omitempty"`
}

// Validate enforces required field constraints.
// Must be called before a Document is considered valid.
func (d *Document) Validate() error {
	if d.ID == "" {
		return ErrMissingID
	}
	if d.Source == "" {
		return ErrMissingSource
	}
	if d.Type == "" {
		return ErrMissingType
	}
	if d.Timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	if d.Text == "" {
		return ErrEmptyText
	}
	return nil
}

// SetExtractedText preserves extracted UTF-8 bytes independently of search normalization.
func (d *Document) SetExtractedText(original, search, version string) {
	d.OriginalText = &original
	d.Text = search
	d.SearchText = search
	d.TextFidelity = "extracted_original"
	d.ExtractionVersion = version
	d.TextSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(original)))
}

// Payload copies metadata before writing protected fields, so source metadata
// cannot replace document identity or evidence.
func (d Document) Payload() map[string]any {
	p := make(map[string]any, len(d.Metadata)+10)
	for k, v := range d.Metadata {
		p[k] = v
	}
	for _, k := range []string{"original_text", "search_text", "text_fidelity", "extraction_version", "text_sha256", "embedding_identity"} {
		delete(p, k)
	}
	p["doc_id"], p["source"], p["type"] = d.ID, d.Source, d.Type
	p["timestamp_unix_ms"], p["text"] = d.Timestamp, d.Text
	p["text_fidelity"] = "legacy_normalized"
	if d.OriginalText != nil {
		p["original_text"], p["search_text"] = *d.OriginalText, d.SearchText
		p["text_fidelity"], p["extraction_version"], p["text_sha256"] = d.TextFidelity, d.ExtractionVersion, d.TextSHA256
	}
	return p
}
