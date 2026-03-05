package qdrant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var ErrInvalidDimension = errors.New("dimension must be > 0")

type createCollectionRequest struct {
	Vectors struct {
		Size     int    `json:"size"`
		Distance string `json:"distance"`
	} `json:"vectors"`
}

func (c *Client) EnsureCollection(ctx context.Context, name string, dim int) error {
	if dim <= 0 {
		return ErrInvalidDimension
	}
	if name == "" {
		return fmt.Errorf("collection name is required")
	}

	var exists any
	err := c.do(ctx, http.MethodGet, "/collections/"+name, nil, &exists)
	if err == nil {
		return nil
	}
	if !isHTTPStatus(err, http.StatusNotFound) {
		return err
	}

	var req createCollectionRequest
	req.Vectors.Size = dim
	req.Vectors.Distance = "Cosine"
	return c.do(ctx, http.MethodPut, "/collections/"+name, req, nil)
}

func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/collections/"+name, nil, nil)
}

func isHTTPStatus(err error, code int) bool {
	s := err.Error()
	return strings.Contains(s, "-> "+strconv.Itoa(code))
}
