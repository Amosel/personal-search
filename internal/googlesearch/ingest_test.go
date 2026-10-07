package googlesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strings"
	"testing"
)

func doc() model.Document {
	d := model.Document{Source: "google_search", Type: "search_query", Timestamp: 1767278400000, Text: "café", Metadata: map[string]any{"account": "personal", "url": "https://www.google.com/search?q=cafe", "raw": map[string]any{"title": "Searched for café"}, "provenance": []any{map[string]any{"locator": "fixture.json", "sha256": strings.Repeat("a", 64), "record_index": float64(0)}}}}
	h := sha256.Sum256([]byte(`["google_search","personal",1767278400000,"café"]`))
	d.ID = hex.EncodeToString(h[:])
	return d
}
func TestLoadBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		archive Archive
		bad     bool
	}{
		{"valid", Archive{1, "google_search", []model.Document{doc()}}, false},
		{"wrong source", Archive{1, "chatgpt", []model.Document{doc()}}, true},
		{"empty", Archive{1, "google_search", nil}, true},
		{"duplicate", Archive{1, "google_search", []model.Document{doc(), doc()}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(tc.archive)
			path := filepath.Join(t.TempDir(), "archive.json")
			os.WriteFile(path, data, 0600)
			_, err := Load(path)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestQdrantWireAndReingest(t *testing.T) {
	stored := map[string]json.RawMessage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":8,"distance":"Cosine"}}}}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/scroll") {
			fmt.Fprint(w, `{"result":{"points":[],"next_page_offset":null}}`)
			return
		}
		if r.URL.Path != "/collections/google_search/points" || r.URL.Query().Get("wait") != "true" {
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(400)
			return
		}
		var body struct {
			Points []struct {
				ID      json.RawMessage `json:"id"`
				Payload map[string]any  `json:"payload"`
				Vector  []float32       `json:"vector"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, point := range body.Points {
			if point.Payload["source"] != "google_search" || point.Payload["text"] != "café" || len(point.Vector) != 8 {
				t.Errorf("invalid point: %+v", point)
			}
			stored[string(point.ID)] = point.ID
		}
		fmt.Fprint(w, `{"result":{"status":"completed"}}`)
	}))
	defer srv.Close()
	for i := 0; i < 2; i++ {
		n, err := Run(context.Background(), []model.Document{doc()}, "google_search", 1, &testEmbedder{embed.FakeEmbedder{DimVal: 8}}, qdrant.New(srv.URL))
		if err != nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
	if len(stored) != 1 {
		t.Fatal("reingest changed point IDs")
	}
}

type failedStore struct{ calls int }

func (s *failedStore) EnsureCollection(context.Context, string, int) error { return nil }
func (s *failedStore) Upsert(context.Context, string, []qdrant.Point) error {
	s.calls++
	if s.calls == 2 {
		return fmt.Errorf("unavailable")
	}
	return nil
}
func TestPartialFailureCount(t *testing.T) {
	second := doc()
	second.Metadata["account"] = "work"
	h := sha256.Sum256([]byte(`["google_search","work",1767278400000,"café"]`))
	second.ID = hex.EncodeToString(h[:])
	n, err := Run(context.Background(), []model.Document{doc(), second}, "google_search", 1, &testEmbedder{embed.FakeEmbedder{DimVal: 8}}, &failedStore{})
	if err == nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

type testEmbedder struct{ embed.FakeEmbedder }

func (*testEmbedder) EmbeddingIdentity() string                                   { return "fake:test:v1" }
func (*failedStore) ValidateEmbedding(context.Context, string, string, int) error { return nil }

func TestArchiveMetadataAndIdentity(t *testing.T) {
	for _, field := range []string{"account", "url", "raw", "provenance"} {
		d := doc()
		delete(d.Metadata, field)
		if err := ValidateDocuments([]model.Document{d}); err == nil {
			t.Fatalf("accepted missing %s", field)
		}
	}
	d := doc()
	d.Metadata["account"] = "another"
	if err := ValidateDocuments([]model.Document{d}); err == nil {
		t.Fatal("accepted wrong account identity")
	}
	d = doc()
	d.Metadata["embedding_identity"] = "foreign"
	if err := ValidateDocuments([]model.Document{d}); err == nil {
		t.Fatal("accepted reserved metadata")
	}
	if got := identityString("<&\u2028\\u2028"); got != "\"<&\u2028\\\\u2028\"" {
		t.Fatalf("canonical quoting: %q", got)
	}
}
