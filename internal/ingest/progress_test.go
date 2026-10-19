package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"testing"
)

var injectedFailure = errors.New("injected batch failure")

type partialStore struct {
	fakeStore
	calls int
}

func (s *partialStore) Upsert(ctx context.Context, c string, p []qdrant.Point) error {
	s.calls++
	if s.calls == 2 {
		return injectedFailure
	}
	return s.fakeStore.Upsert(ctx, c, p)
}
func threeMessageExport(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.json")
	data := `{"conversations":[{"id":"c","mapping":{"a":{"id":"a","message":{"author":{"role":"user"},"create_time":1,"content":{"parts":["first"]}}},"b":{"id":"b","message":{"author":{"role":"user"},"create_time":2,"content":{"parts":["second"]}}},"c":{"id":"c","message":{"author":{"role":"user"},"create_time":3,"content":{"parts":["third"]}}}}}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestRunKeepsAcknowledgedProgressAfterSecondBatchFailure(t *testing.T) {
	store := &partialStore{}
	r, err := Run(context.Background(), Options{ExportPath: threeMessageExport(t), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "test", BatchSize: 1}, &embed.FakeEmbedder{DimVal: 3}, store)
	if !errors.Is(err, injectedFailure) {
		t.Fatalf("%v", err)
	}
	if r.Documents != 1 {
		t.Fatalf("acknowledged=%d want=1", r.Documents)
	}
}

type faultEmbedder struct {
	embed.FakeEmbedder
	calls  int
	mode   string
	cancel context.CancelFunc
}

func (e *faultEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	if e.calls == 2 {
		switch e.mode {
		case "error":
			return nil, injectedFailure
		case "count":
			return nil, nil
		case "dimension":
			return [][]float32{{1}}, nil
		case "cancel":
			e.cancel()
			return e.FakeEmbedder.Embed(ctx, texts)
		}
	}
	return e.FakeEmbedder.Embed(ctx, texts)
}
func TestRunFailureStagesKeepProgressAndReports(t *testing.T) {
	for _, mode := range []string{"error", "count", "dimension", "cancel", "upsert", "report", "joined"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			reportPath := filepath.Join(dir, "report.json")
			if mode == "report" || mode == "joined" {
				reportPath = filepath.Join(dir, "missing", "report.json")
			}
			emb := &faultEmbedder{FakeEmbedder: embed.FakeEmbedder{DimVal: 3}, mode: mode, cancel: cancel}
			var store Store = &fakeStore{}
			if mode == "upsert" || mode == "joined" {
				store = &partialStore{}
			}
			r, err := Run(ctx, Options{ExportPath: threeMessageExport(t), ReportPath: reportPath, Collection: "test", BatchSize: 1}, emb, store)
			if err == nil || r.Report.Status != "failed" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			ack, uncertain, notAttempted := 1, 0, 2
			if mode == "report" {
				ack, notAttempted = 3, 0
			}
			if mode == "upsert" || mode == "joined" {
				uncertain, notAttempted = 1, 1
			}
			p := r.Progress
			if r.Documents != ack || p.Acknowledged != ack || p.Uncertain != uncertain || p.NotAttempted != notAttempted || p.Selected != 3 || p.Classified != 3 {
				t.Fatalf("progress=%+v", p)
			}
			var operation *model.IngestError
			if !errors.As(err, &operation) {
				t.Fatalf("missing typed cause %v", err)
			}
			if uncertain > 0 && (len(p.UncertainBatches) != 1 || p.UncertainBatches[0] != (model.BatchRange{Start: 1, End: 2})) {
				t.Fatalf("uncertain=%v", p.UncertainBatches)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation %v", err)
			}
			if mode == "joined" {
				var pathError *os.PathError
				if !errors.Is(err, injectedFailure) || !errors.As(err, &pathError) {
					t.Fatalf("lost joined causes %v", err)
				}
			}
			if mode != "report" && mode != "joined" {
				data, readErr := os.ReadFile(reportPath)
				if readErr != nil {
					t.Fatal(readErr)
				}
				var saved chatgpt.IngestReport
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				if saved.Status != "failed" || saved.SchemaVersion != 2 || saved.Progress.Acknowledged != ack || saved.Failure.Batch.End != 2 {
					t.Fatalf("saved=%+v", saved)
				}
				info, _ := os.Stat(reportPath)
				if info.Mode().Perm() != 0600 {
					t.Fatalf("report mode=%v", info.Mode())
				}
			}
		})
	}
}

type keyedStore struct {
	fakeStore
	points map[qdrant.PointID]qdrant.Point
}

func (s *keyedStore) Upsert(_ context.Context, _ string, p []qdrant.Point) error {
	for _, point := range p {
		s.points[point.ID] = point
	}
	return nil
}
func TestRunFullRetryAndMaxDocs(t *testing.T) {
	path := threeMessageExport(t)
	store := &keyedStore{points: map[qdrant.PointID]qdrant.Point{}}
	for _, limit := range []int{2, 0, 0} {
		r, err := Run(context.Background(), Options{ExportPath: path, ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "test", BatchSize: 2, MaxDocs: limit}, &embed.FakeEmbedder{DimVal: 3}, store)
		want := 3
		if limit > 0 {
			want = limit
		}
		if err != nil || r.Documents != want || len(store.points) != want || r.Progress.NotAttempted != 0 || r.Progress.Excluded != 3-want {
			t.Fatalf("result=%+v stored=%d err=%v", r, len(store.points), err)
		}
	}
}

type cancelAfterStore struct {
	fakeStore
	cancel context.CancelFunc
}

func (s *cancelAfterStore) Upsert(ctx context.Context, c string, p []qdrant.Point) error {
	if err := s.fakeStore.Upsert(ctx, c, p); err != nil {
		return err
	}
	s.cancel()
	return nil
}
func TestCancellationAfterFinalAcknowledgementKeepsCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &cancelAfterStore{cancel: cancel}
	r, err := Run(ctx, Options{ExportPath: threeMessageExport(t), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "synthetic", BatchSize: 3}, &embed.FakeEmbedder{DimVal: 3}, store)
	if !errors.Is(err, context.Canceled) || r.Documents != 3 || r.Progress.NotAttempted != 0 || r.Progress.Uncertain != 0 || r.Report.Status != "failed" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
