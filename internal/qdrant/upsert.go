package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

type Point struct {
	ID      any            `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

type upsertRequest struct {
	Points []Point `json:"points"`
}

func (c *Client) Upsert(ctx context.Context, collection string, points []Point) error {
	if collection == "" {
		return fmt.Errorf("collection is required")
	}
	if len(points) == 0 {
		return fmt.Errorf("points are required")
	}
	dim := len(points[0].Vector)
	if dim == 0 {
		return fmt.Errorf("point vector dimension must be > 0")
	}
	for i, p := range points {
		if !validPointID(p.ID) {
			return fmt.Errorf("point %d: id is required and must be string/int/uint", i)
		}
		if len(p.Vector) != dim {
			return fmt.Errorf("point %d: dimension mismatch got=%d want=%d", i, len(p.Vector), dim)
		}
	}

	req := upsertRequest{Points: points}
	return c.do(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", req, nil)
}

func validPointID(v any) bool {
	switch id := v.(type) {
	case string:
		return id != ""
	case int:
		return id >= 0
	case int32:
		return id >= 0
	case int64:
		return id >= 0
	case uint:
		return true
	case uint32:
		return true
	case uint64:
		return true
	default:
		return false
	}
}
