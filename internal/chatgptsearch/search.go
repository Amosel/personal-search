package chatgptsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"personal-search/internal/model"
)

type Options struct {
	Query    string
	Limit    int
	Author   string
	ThreadID string
	From     string
	To       string
}

type Searcher interface {
	Search(ctx context.Context, req model.SearchRequest) (model.SearchResponse, error)
}

type HTTPClient struct {
	BaseURL string
	Client  *http.Client
}

func BuildRequest(opts Options) model.SearchRequest {
	filters := &model.Filters{
		Source: []string{"chatgpt"},
	}
	if opts.Author != "" {
		filters.Author = opts.Author
	}
	if opts.ThreadID != "" {
		filters.ThreadID = opts.ThreadID
	}
	if opts.From != "" || opts.To != "" {
		filters.Date = &model.DateRange{
			From: opts.From,
			To:   opts.To,
		}
	}
	return model.SearchRequest{
		Query:   opts.Query,
		Filters: filters,
		Limit:   opts.Limit,
	}
}

func (c *HTTPClient) Search(ctx context.Context, req model.SearchRequest) (model.SearchResponse, error) {
	var out model.SearchResponse
	if c.BaseURL == "" {
		return out, fmt.Errorf("base URL is required")
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return out, fmt.Errorf("marshal request: %w", err)
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/search"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return out, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return out, fmt.Errorf("search failed: %s", strings.TrimSpace(string(msg)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}
