package ingest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/ingestreport"
	"personal-search/internal/model"
	"testing"
)

type fakeStore struct {
	ensured    int
	upserts    [][]model.Document
	vectors    [][][]float32
	failUpsert bool
}

func (s *fakeStore) EnsureCollection(_ context.Context, _ string, dim int) error {
	if dim != 3 {
		return os.ErrInvalid
	}
	s.ensured++
	return nil
}
func (s *fakeStore) Upsert(_ context.Context, _ string, docs []model.Document, vectors [][]float32) error {
	if s.failUpsert {
		return os.ErrInvalid
	}
	s.upserts = append(s.upserts, docs)
	s.vectors = append(s.vectors, vectors)
	return nil
}

type fixtureSourceAdapter struct {
	path string
	docs []model.Document
}

type fixtureReport struct {
	state  string
	counts ingestreport.Summary
}

func (r *fixtureReport) MarkFailed(_, _ string)       { r.state = "failed" }
func (r *fixtureReport) MarkCompleted()               { r.state = "completed" }
func (r *fixtureReport) Counts() ingestreport.Summary { return r.counts }
func (r *fixtureReport) ReportState() string          { return r.state }
func (r *fixtureReport) Write(path string) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (a *fixtureSourceAdapter) LoadDocuments(path string) ([]model.Document, ingestreport.Report, error) {
	a.path = path
	return a.docs, &fixtureReport{counts: ingestreport.Summary{TotalRecords: len(a.docs), DocumentsCreated: len(a.docs)}}, nil
}

func TestRunWithSourceAdapterContract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.input")
	doc := model.Document{
		ID: "fixture:record-1", Source: "fixture", Type: "record", Timestamp: 1234,
		Text: "fixture content", Metadata: map[string]any{"external_id": "record-1"},
	}
	adapter := &fixtureSourceAdapter{docs: []model.Document{doc}}
	store := &fakeStore{}
	result, err := Run(context.Background(), Options{
		InputPath: path, ReportPath: filepath.Join(dir, "report.json"), Collection: "docs", BatchSize: 1,
	}, adapter, &embed.FakeEmbedder{DimVal: 3}, store)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.path != path {
		t.Fatalf("adapter input = %q, want %q", adapter.path, path)
	}
	if result.Documents != 1 || len(store.upserts) != 1 || len(store.upserts[0]) != 1 {
		t.Fatalf("source documents not ingested: result=%+v store=%+v", result, store)
	}
	if result.Report.ReportState() != "completed" || result.Report.Counts().DocumentsCreated != 1 {
		t.Fatalf("source report lifecycle/counts not preserved: state=%q counts=%+v", result.Report.ReportState(), result.Report.Counts())
	}
	got := store.upserts[0][0]
	if got.ID != doc.ID || got.Source != doc.Source || got.Text != doc.Text || got.Metadata["external_id"] != "record-1" {
		t.Fatalf("canonical document provenance was not preserved across storage boundary: %+v", got)
	}
	if len(store.vectors[0][0]) != 3 {
		t.Fatalf("embedding dimension = %d, want 3", len(store.vectors[0][0]))
	}
}

func TestRunUsesRealExportConversionAndDurableStore(t *testing.T) {
	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	data := `{"conversations":[{"id":"c1","title":"t","mapping":{"m1":{"id":"m1","message":{"author":{"role":"user"},"create_time":1234567890,"content":{"content_type":"text","parts":["hello search"]}}}}}]}`
	if err := os.WriteFile(export, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{}
	result, err := Run(context.Background(), Options{InputPath: export, ReportPath: filepath.Join(dir, "report.json"), Collection: "docs", BatchSize: 1}, chatgpt.SourceAdapter{}, &embed.FakeEmbedder{DimVal: 3}, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Documents != 1 || result.Report.ReportState() != "completed" || store.ensured != 1 || len(store.upserts) != 1 || len(store.upserts[0]) != 1 {
		t.Fatalf("unexpected result: %+v store=%+v", result, store)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunFailureIsNotCompleted(t *testing.T) {
	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	if err := os.WriteFile(export, []byte(`{"conversations":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Options{InputPath: export, ReportPath: filepath.Join(dir, "report.json"), Collection: "docs", BatchSize: 1}, chatgpt.SourceAdapter{}, &embed.FakeEmbedder{DimVal: 3}, &fakeStore{})
	if err == nil || result.Report == nil || result.Report.ReportState() != "failed" {
		t.Fatalf("expected failed report, result=%+v err=%v", result, err)
	}
}

func TestDefaultReportPathIsWorkingDirectoryRelative(t *testing.T) {
	if got, want := DefaultReportPath(), filepath.Join(".", "ingest_report.json"); got != want {
		t.Fatalf("default report path = %q, want %q", got, want)
	}
}
