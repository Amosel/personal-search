package model

import "errors"

var (
	ErrMissingID        = errors.New("document ID is required")
	ErrMissingSource    = errors.New("document source is required")
	ErrMissingType      = errors.New("document type is required")
	ErrInvalidTimestamp = errors.New("document timestamp must be > 0")
	ErrEmptyText        = errors.New("document text is required")
)

// Document is the canonical unit of search.
type Document struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Type      string         `json:"type"`
	Timestamp int64          `json:"timestamp_unix_ms"`
	Text      string         `json:"text"`
	Metadata  map[string]any `json:"metadata"`
	Keywords  []string       `json:"keywords,omitempty"`
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
