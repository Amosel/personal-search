package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"personal-search/internal/embed"
	"personal-search/internal/googlesearch"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in. Uses labeled synthetic records; retains its new collection.
// Never run the repository's destructive integration cleanup against user data.
func TestGoogleLiveSynthetic(t *testing.T) {
	if os.Getenv("GOOGLE_SEARCH_LIVE") != "1" {
		t.Skip("opt in with GOOGLE_SEARCH_LIVE=1; creates and retains a synthetic verification collection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	emb, err := googlesearch.NewEmbedder(ctx, embed.DefaultOllamaURL, embed.DefaultOllamaModel)
	if err != nil {
		t.Fatal(err)
	}
	collection := fmt.Sprintf("google_search_synthetic_verify_%d", time.Now().UnixNano())
	docs := []model.Document{}
	for i, text := range []string{"synthetic google integration known record", "synthetic second record", "synthetic excluded date"} {
		timestamp := int64(1767278400123)
		if i == 2 {
			timestamp += 86400000
		}
		account := "synthetic-verification"
		url := "https://www.google.com/search?q=synthetic"
		identity, _ := json.Marshal([]any{"google_search", account, timestamp, text})
		hash := sha256.Sum256(identity)
		docs = append(docs, model.Document{ID: hex.EncodeToString(hash[:]), Source: "google_search", Type: "search_query", Timestamp: timestamp, Text: text, Metadata: map[string]any{"account": account, "url": url, "raw": map[string]any{"title": "Searched for " + text}, "provenance": []any{map[string]any{"locator": "synthetic-fixture", "sha256": strings.Repeat("a", 64), "record_index": float64(i)}}}})
	}
	archivePath := filepath.Join(t.TempDir(), "synthetic.json")
	archive, _ := json.Marshal(googlesearch.Archive{SchemaVersion: 1, Source: "google_search", Documents: docs})
	if err := os.WriteFile(archivePath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	docs, err = googlesearch.Load(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	qc := qdrant.New("http://localhost:6333")
	n, err := googlesearch.Run(ctx, docs, collection, 2, emb, qc)
	if err != nil || n != 3 {
		t.Fatalf("import acknowledged=%d err=%v collection=%s", n, err, collection)
	}
	before, err := qc.Count(ctx, collection)
	if err != nil || before != 3 {
		t.Fatalf("count=%d err=%v", before, err)
	}
	// The ChatGPT collection is not searched or modified by this test.
	mux := newMux(qc, &embed.FakeEmbedder{DimVal: 2}, "chatgpt_messages", collection, embed.DefaultOllamaURL, embed.DefaultOllamaModel)
	search := func(source, from, to string) model.SearchResponse {
		t.Helper()
		body, _ := json.Marshal(model.SearchRequest{Query: docs[0].Text, Limit: 10, Filters: &model.Filters{Source: []string{source}, Date: &model.DateRange{From: from, To: to}}})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/search", strings.NewReader(string(body))).WithContext(ctx))
		if w.Code != 200 {
			t.Fatalf("search status=%d body=%s", w.Code, w.Body.String())
		}
		var out model.SearchResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := search("google_search", "2026-01-01", "2026-01-01")
	if len(out.Results) != 2 || out.Results[0].ID != docs[0].ID {
		t.Fatalf("known-record/date retrieval: %+v", out)
	}
	for _, r := range out.Results {
		if r.Source != "google_search" || r.Timestamp != "2026-01-01T14:40:00.123Z" || r.Metadata["account"] != "synthetic-verification" || r.Metadata["url"] != docs[0].Metadata["url"] || r.Metadata["provenance"] == nil {
			t.Fatalf("metadata lost: %+v", r)
		}
	}
	if out := search("google_search", "2026-01-03", "2026-01-03"); len(out.Results) != 0 {
		t.Fatal("excluded date returned results")
	}
	vectors, err := emb.Embed(ctx, []string{docs[0].Text})
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := qc.Search(ctx, collection, qdrant.SearchRequest{Vector: vectors[0], Limit: 10, WithPayload: true, Filter: buildQdrantFilter(&model.Filters{Source: []string{"chatgpt"}})})
	if err != nil || len(excluded.Result) != 0 {
		t.Fatalf("source exclusion failed: %+v %v", excluded, err)
	}
	n, err = googlesearch.Run(ctx, docs, collection, 2, emb, qc)
	if err != nil || n != 3 {
		t.Fatalf("reimport=%d err=%v", n, err)
	}
	after, err := qc.Count(ctx, collection)
	if err != nil || after != before {
		t.Fatalf("idempotency before=%d after=%d err=%v", before, after, err)
	}
	if err := qc.ValidateEmbedding(ctx, collection, "ollama:incompatible:clip2000:v1", emb.Dim()); err == nil {
		t.Fatal("same-dimension model mismatch accepted")
	}
	t.Logf("LIVE SYNTHETIC: collection=%s unique_points=%d first_import_acknowledged=3 reimport_acknowledged=3 known_record=true source_and_date_filters=true model_mismatch_rejected=true", collection, after)
}
