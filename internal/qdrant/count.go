package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

type countRequest struct {
	Exact bool `json:"exact"`
}

type countResponse struct {
	Result struct {
		Count int `json:"count"`
	} `json:"result"`
}

func (c *Client) Count(ctx context.Context, collection string) (int, error) {
	if collection == "" {
		return 0, fmt.Errorf("collection is required")
	}

	var out countResponse
	err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/count", countRequest{Exact: true}, &out)
	if err != nil {
		return 0, err
	}
	return out.Result.Count, nil
}
