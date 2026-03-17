package main

import (
	"context"
	"encoding/json"
	"testing"

	"personal-search/internal/model"
)

func TestHandleInitialize(t *testing.T) {
	h := Handler{}
	resp, err := h.handle(context.Background(), rpcRequest{Method: "initialize"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp["protocolVersion"] == nil {
		t.Fatalf("missing protocol version: %+v", resp)
	}
}

func TestHandleToolsList(t *testing.T) {
	h := Handler{}
	resp, err := h.handle(context.Background(), rpcRequest{Method: "tools/list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := json.Marshal(resp)
	if string(raw) == "{}" {
		t.Fatal("expected tool listing")
	}
}

func TestHandleToolCall(t *testing.T) {
	h := Handler{
		searcher: fakeSearcher{
			resp: model.SearchResponse{
				Results: []model.SearchResult{{ID: "1", Source: "chatgpt", Text: "match"}},
			},
		},
	}
	resp, err := h.handle(context.Background(), rpcRequest{
		Method: "tools/call",
		Params: mustJSON(map[string]any{
			"name": "search_documents",
			"arguments": map[string]any{
				"query":  "match",
				"limit":  3,
				"author": "user",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := json.Marshal(resp)
	if len(raw) == 0 {
		t.Fatal("expected tool call response")
	}
}

func TestHandleToolCall_RejectsUnknownTool(t *testing.T) {
	h := Handler{}
	_, err := h.handle(context.Background(), rpcRequest{
		Method: "tools/call",
		Params: mustJSON(map[string]any{
			"name":      "wrong_tool",
			"arguments": map[string]any{"query": "match"},
		}),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

type fakeSearcher struct {
	resp model.SearchResponse
	err  error
}

func (f fakeSearcher) Search(_ context.Context, _ model.SearchRequest) (model.SearchResponse, error) {
	return f.resp, f.err
}
