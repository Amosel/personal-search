package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"personal-search/internal/embed"
	"personal-search/internal/ingest"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

type fidelityStore struct{ points []qdrant.Point }

func (*fidelityStore) EnsureCollection(context.Context, string, int) error { return nil }
func (s *fidelityStore) Upsert(_ context.Context, _ string, p []qdrant.Point) error {
	s.points = append(s.points, p...)
	return nil
}
func TestOriginalTextSurvivesIngestAndThreadHTTP(t *testing.T) {
	original := "def f():\r\n    return 'שלום 🌍'  \r\n\r\nroot:\n  child: value\n"
	content, _ := json.Marshal(original)
	dir := t.TempDir()
	path := filepath.Join(dir, "export.json")
	data := `{"conversations":[{"id":"c","mapping":{"m":{"id":"m","message":{"author":{"role":"user"},"create_time":1234567890,"content":{"parts":[` + string(content) + `]}}}}}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store := &fidelityStore{}
	_, err := ingest.Run(context.Background(), ingest.Options{ExportPath: path, ReportPath: filepath.Join(dir, "report.json"), Collection: "test", BatchSize: 1}, &embed.FakeEmbedder{DimVal: 3}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"points": []any{store.points[0], map[string]any{"id": 1, "payload": map[string]any{"text": "old normalized", "timestamp_unix_ms": 1234567890001}}}, "next_page_offset": nil}})
	}))
	defer server.Close()
	mux := newMux(qdrant.New(server.URL), &embed.FakeEmbedder{DimVal: 3}, "test", "google", "", "")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/thread?thread_id=c", nil))
	var body struct {
		Results []model.SearchResult `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Results) != 2 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if body.Results[0].Text != original || body.Results[0].TextFidelity != "extracted_original" {
		t.Fatalf("lost evidence: %+v", body.Results[0])
	}
	if body.Results[1].Text != "old normalized" || body.Results[1].TextFidelity != "legacy_normalized" {
		t.Fatalf("legacy mislabeled: %+v", body.Results[1])
	}
}
