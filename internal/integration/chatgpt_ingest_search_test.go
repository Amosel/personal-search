package integration

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"personal-search/internal/chatgpt"
	"personal-search/internal/qdrant"
)

func TestChatGPTIngestThenSearch(t *testing.T) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}

	ctx := context.Background()
	qc := qdrant.New(qdrantURL)
	if err := qc.Healthy(ctx); err != nil {
		t.Skipf("qdrant not reachable at %s: %v", qdrantURL, err)
	}

	fixture := filepath.Join("testdata", "chatgpt_export_valid.json")
	exp, err := chatgpt.LoadExport(fixture)
	if err != nil {
		t.Fatalf("load export: %v", err)
	}
	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		t.Fatalf("to documents: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected documents from fixture")
	}

	emb := fakeEmbedder{dim: 16}

	collection := fmt.Sprintf("itest_chatgpt_%d", time.Now().UnixNano())
	if err := qc.EnsureCollection(ctx, collection, emb.Dim()); err != nil {
		t.Fatalf("ensure collection: %v", err)
	}
	t.Cleanup(func() {
		_ = qc.DeleteCollection(context.Background(), collection)
	})

	texts := make([]string, len(docs))
	for i := range docs {
		texts[i] = docs[i].Text
	}

	vectors, err := emb.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed docs: %v", err)
	}

	points := make([]qdrant.Point, 0, len(docs))
	for i, d := range docs {
		payload := map[string]any{
			"source":            d.Source,
			"type":              d.Type,
			"timestamp_unix_ms": float64(d.Timestamp),
			"text":              d.Text,
		}
		for k, v := range d.Metadata {
			payload[k] = v
		}
		points = append(points, qdrant.Point{
			ID:      qdrantNumericID(d.ID),
			Vector:  vectors[i],
			Payload: payload,
		})
	}

	if err := qc.Upsert(ctx, collection, points); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	queryVecs, err := emb.Embed(ctx, []string{"custody strategy planning notes"})
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}

	out, err := qc.Search(ctx, collection, qdrant.SearchRequest{
		Vector:      queryVecs[0],
		Limit:       3,
		WithPayload: true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out.Result) == 0 {
		t.Fatal("expected search results after insert")
	}

	top := out.Result[0]
	if top.Payload == nil {
		t.Fatal("expected payload in search result")
	}
	if _, ok := top.Payload["thread_id"]; !ok {
		t.Fatal("expected thread_id in payload")
	}
	if _, ok := top.Payload["author"]; !ok {
		t.Fatal("expected author in payload")
	}
}

type fakeEmbedder struct {
	dim int
}

func (f fakeEmbedder) Dim() int {
	return f.dim
}

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("text at index %d is empty", i)
		}
		vec := make([]float32, f.dim)
		for _, tok := range splitWords(text) {
			h := fnv.New64a()
			_, _ = h.Write([]byte(tok))
			idx := int(h.Sum64() % uint64(f.dim))
			vec[idx]++
		}
		out[i] = vec
	}
	return out, nil
}

func splitWords(s string) []string {
	parts := make([]string, 0, len(s)/4)
	start := -1
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			parts = append(parts, s[start:i])
			start = -1
		}
	}
	if start != -1 {
		parts = append(parts, s[start:])
	}
	return parts
}

func qdrantNumericID(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
