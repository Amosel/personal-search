package chatgptsearch

import (
	"testing"
)

func TestBuildRequest_ScopesToChatGPT(t *testing.T) {
	req := BuildRequest(Options{Query: "custody strategy", Limit: 7})

	if req.Query != "custody strategy" {
		t.Fatalf("unexpected query: %q", req.Query)
	}
	if req.Limit != 7 {
		t.Fatalf("unexpected limit: %d", req.Limit)
	}
	if req.Filters == nil || len(req.Filters.Source) != 1 || req.Filters.Source[0] != "chatgpt" {
		t.Fatalf("expected chatgpt source filter, got %+v", req.Filters)
	}
}

func TestBuildRequest_PreservesSupportedFilters(t *testing.T) {
	req := BuildRequest(Options{
		Query:    "planning",
		Author:   "user",
		ThreadID: "conv-alpha",
		From:     "2024-12-31",
		To:       "2024-12-31",
	})

	if req.Filters == nil {
		t.Fatal("expected filters")
	}
	if req.Filters.Author != "user" {
		t.Fatalf("unexpected author: %q", req.Filters.Author)
	}
	if req.Filters.ThreadID != "conv-alpha" {
		t.Fatalf("unexpected thread_id: %q", req.Filters.ThreadID)
	}
	if req.Filters.Date == nil || req.Filters.Date.From != "2024-12-31" || req.Filters.Date.To != "2024-12-31" {
		t.Fatalf("unexpected date filter: %+v", req.Filters.Date)
	}
}
