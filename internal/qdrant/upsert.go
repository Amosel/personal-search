package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

type Point struct {
	ID      PointID        `json:"id"`
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
		if len(p.Vector) != dim {
			return fmt.Errorf("point %d: dimension mismatch got=%d want=%d", i, len(p.Vector), dim)
		}
	}

	req := upsertRequest{Points: points}
	var out struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", req, &out); err != nil {
		return err
	}
	if out.Result.Status != "completed" {
		return fmt.Errorf("upsert completion unconfirmed: status=%q; batch may have been applied", out.Result.Status)
	}
	return nil
}
