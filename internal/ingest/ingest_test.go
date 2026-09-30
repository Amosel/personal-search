package ingest

import (
	"context"
	"os"
	"path/filepath"
	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
	"testing"
)

type fakeStore struct {
	ensured    int
	upserts    [][]qdrant.Point
	failUpsert bool
}

func (s *fakeStore) EnsureCollection(_ context.Context, _ string, dim int) error {
	if dim != 3 {
		return os.ErrInvalid
	}
	s.ensured++
	return nil
}
func (s *fakeStore) Upsert(_ context.Context, _ string, points []qdrant.Point) error {
	if s.failUpsert {
		return os.ErrInvalid
	}
	s.upserts = append(s.upserts, points)
	return nil
}

func TestRunUsesRealExportConversionAndDurableStore(t *testing.T) {
	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	data := `{"conversations":[{"id":"c1","title":"t","mapping":{"m1":{"id":"m1","message":{"author":{"role":"user"},"create_time":1234567890,"content":{"content_type":"text","parts":["hello search"]}}}}}]}`
	if err := os.WriteFile(export, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{}
	result, err := Run(context.Background(), Options{ExportPath: export, ReportPath: filepath.Join(dir, "report.json"), Collection: "docs", BatchSize: 1}, &embed.FakeEmbedder{DimVal: 3}, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Documents != 1 || result.Report.Status != "completed" || store.ensured != 1 || len(store.upserts) != 1 || len(store.upserts[0]) != 1 {
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
	result, err := Run(context.Background(), Options{ExportPath: export, ReportPath: filepath.Join(dir, "report.json"), Collection: "docs", BatchSize: 1}, &embed.FakeEmbedder{DimVal: 3}, &fakeStore{})
	if err == nil || result.Report == nil || result.Report.Status != "failed" {
		t.Fatalf("expected failed report, result=%+v err=%v", result, err)
	}
}

func TestDefaultReportPathIsWorkingDirectoryRelative(t *testing.T) {
	if got, want := DefaultReportPath(), filepath.Join(".", "ingest_report.json"); got != want {
		t.Fatalf("default report path = %q, want %q", got, want)
	}
}
