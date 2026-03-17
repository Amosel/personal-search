package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"personal-search/internal/chatgptsearch"
	"personal-search/internal/model"
)

func TestRun_RequiresQuery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{}, &stdout, &stderr, fakeSearcher{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_EmitsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--query", "custody"}, &stdout, &stderr, fakeSearcher{
		resp: model.SearchResponse{
			Results: []model.SearchResult{{ID: "1", Source: "chatgpt", Text: "custody strategy"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"source":"chatgpt"`) {
		t.Fatalf("expected JSON output, got %s", stdout.String())
	}
}

func TestRun_EmitsYAML(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--query", "custody", "--format", "yaml"}, &stdout, &stderr, fakeSearcher{
		resp: model.SearchResponse{
			Results: []model.SearchResult{{
				ID:        "1",
				Source:    "chatgpt",
				Text:      "line one\nline two",
				Timestamp: "2025-01-01T00:00:00Z",
				Metadata: map[string]any{
					"author": "assistant",
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "results:") || !strings.Contains(out, `source: "chatgpt"`) {
		t.Fatalf("expected YAML output, got %s", out)
	}
	if !strings.Contains(out, "text: |-\n") {
		t.Fatalf("expected YAML block scalar, got %s", out)
	}
	if !strings.Contains(out, "\n      line one\n") {
		t.Fatalf("expected properly indented multiline text, got %s", out)
	}
}

type fakeSearcher struct {
	resp model.SearchResponse
	err  error
	req  model.SearchRequest
}

func (f fakeSearcher) Search(_ context.Context, req model.SearchRequest) (model.SearchResponse, error) {
	f.req = req
	return f.resp, f.err
}

var _ chatgptsearch.Searcher = fakeSearcher{}
