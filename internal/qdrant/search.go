package qdrant

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

type MatchValue struct {
	Value any `json:"value"`
}

type Range struct {
	Gte *float64 `json:"gte,omitempty"`
	Lte *float64 `json:"lte,omitempty"`
}

type FieldCondition struct {
	Key   string      `json:"key"`
	Match *MatchValue `json:"match,omitempty"`
	Range *Range      `json:"range,omitempty"`
}

type Filter struct {
	Must []any `json:"must,omitempty"`
}

type SearchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
	Filter      *Filter   `json:"filter,omitempty"`
}

type SearchResponse struct {
	Result []ScoredPoint `json:"result"`
}

type ScoredPoint struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func (c *Client) Search(ctx context.Context, collection string, req SearchRequest) (*SearchResponse, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection is required")
	}
	if len(req.Vector) == 0 {
		return nil, fmt.Errorf("search vector is required")
	}
	if req.Limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	var out SearchResponse
	if err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/search", req, &out); err != nil {
		return nil, err
	}

	sort.Slice(out.Result, func(i, j int) bool {
		return out.Result[i].Score > out.Result[j].Score
	})
	return &out, nil
}
