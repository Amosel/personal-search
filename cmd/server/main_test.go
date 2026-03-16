package main

import (
	"testing"

	"personal-search/internal/model"
)

func TestBuildQdrantFilter_SourceAuthorThreadDate(t *testing.T) {
	f := &model.Filters{
		Source:   []string{"chatgpt", "gmail"},
		Author:   "user",
		ThreadID: "thread-1",
		Date: &model.DateRange{
			From: "2024-01-01",
			To:   "2024-01-31",
		},
	}

	got := buildQdrantFilter(f)
	if got == nil {
		t.Fatal("expected filter")
	}
	if len(got.Should) != 2 {
		t.Fatalf("expected 2 source should clauses, got %d", len(got.Should))
	}
	if len(got.Must) != 3 {
		t.Fatalf("expected 3 must clauses, got %d", len(got.Must))
	}
}

func TestBuildQdrantFilter_EmptySourceIgnored(t *testing.T) {
	f := &model.Filters{
		Source: []string{"", "chatgpt"},
	}

	got := buildQdrantFilter(f)
	if got == nil {
		t.Fatal("expected filter")
	}
	if len(got.Should) != 1 {
		t.Fatalf("expected 1 source clause, got %d", len(got.Should))
	}
}

func TestSearchRequestValidate_RejectsKeywords(t *testing.T) {
	req := &model.SearchRequest{
		Query: "test",
		Filters: &model.Filters{
			Keywords: []string{"custody"},
		},
	}

	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}
