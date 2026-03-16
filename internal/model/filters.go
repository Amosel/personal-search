package model

import (
	"fmt"
	"strings"
	"time"
)

type DateRange struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

func (d *DateRange) Validate() error {
	if d == nil {
		return nil
	}
	if d.From != "" {
		if _, err := time.Parse("2006-01-02", d.From); err != nil {
			return fmt.Errorf("invalid date.from: %w", err)
		}
	}
	if d.To != "" {
		if _, err := time.Parse("2006-01-02", d.To); err != nil {
			return fmt.Errorf("invalid date.to: %w", err)
		}
	}
	return nil
}

type Filters struct {
	Source   []string   `json:"source,omitempty"`
	Date     *DateRange `json:"date,omitempty"`
	Keywords []string   `json:"keywords,omitempty"`
	Author   string     `json:"author,omitempty"`
	ThreadID string     `json:"thread_id,omitempty"`
}

func (f *Filters) Validate() error {
	if f == nil {
		return nil
	}
	if err := f.Date.Validate(); err != nil {
		return err
	}
	if len(f.Keywords) > 0 {
		return fmt.Errorf("keywords filter is not supported")
	}
	return nil
}

type SearchRequest struct {
	Query   string   `json:"query"`
	Filters *Filters `json:"filters,omitempty"`
	Limit   int      `json:"limit,omitempty"`
}

func (r *SearchRequest) Validate() error {
	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("query is required")
	}
	if r.Filters != nil {
		if err := r.Filters.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SearchResult struct {
	ID        string         `json:"id"`
	Score     float64        `json:"score"`
	Text      string         `json:"text"`
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}
