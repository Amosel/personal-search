package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
)

var errUpsert = errors.New("upsert completion lost")

type progressStore struct {
	stored      map[any]qdrant.Point
	calls       int
	failAt      int
	upsertErr   error
	ensureErr   error
	ensureCalls int
	batchSizes  []int
	afterUpsert func(int)
}

func (s *progressStore) EnsureCollection(context.Context, string, int) error {
	s.ensureCalls++
	return s.ensureErr
}

func (s *progressStore) Upsert(_ context.Context, _ string, points []qdrant.Point) error {
	s.calls++
	s.batchSizes = append(s.batchSizes, len(points))
	if s.stored == nil {
		s.stored = make(map[any]qdrant.Point)
	}
	for _, p := range points {
		s.stored[p.ID] = p
	}
	if s.afterUpsert != nil {
		s.afterUpsert(s.calls)
	}
	if s.calls == s.failAt {
		if s.upsertErr != nil {
			return s.upsertErr
		}
		return errUpsert
	}
	return nil
}

func TestRunCancellationDuringUpsertIsUncertain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &progressStore{failAt: 2, upsertErr: context.Canceled, afterUpsert: func(call int) {
		if call == 2 {
			cancel()
		}
	}}
	result, err := Run(ctx, Options{ExportPath: syntheticExport(t, 5), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "synthetic", BatchSize: 2}, &embed.FakeEmbedder{DimVal: 3}, store)
	if !errors.Is(err, context.Canceled) || result.Report.Status != "failed" || result.Report.Failure.Stage != "upsert" || store.calls != 2 || len(store.stored) != 4 {
		t.Fatalf("result=%+v err=%v store=%+v", result, err, store)
	}
	assertProgress(t, result, 5, 2, 2, 1)
}

func syntheticExport(t *testing.T, n int) string {
	t.Helper()
	conv := chatgpt.Conversation{ID: "synthetic", Mapping: make(map[string]chatgpt.Message)}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m%d", i)
		conv.Mapping[id] = chatgpt.Message{ID: id, Message: &chatgpt.MsgBody{
			Author: chatgpt.Author{Role: "user"}, CreateTime: float64(i + 1),
			Content: chatgpt.Content{ContentType: "text", Parts: []any{fmt.Sprintf("synthetic message %d", i)}},
		}}
	}
	data, err := json.Marshal(chatgpt.Export{Conversations: []chatgpt.Conversation{conv}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunPartialUpsertKeepsAcknowledgedDocuments(t *testing.T) {
	store := &progressStore{failAt: 2}
	result, err := Run(context.Background(), Options{
		ExportPath: syntheticExport(t, 5), ReportPath: filepath.Join(t.TempDir(), "report.json"),
		Collection: "synthetic", BatchSize: 2,
	}, &embed.FakeEmbedder{DimVal: 3}, store)
	if !errors.Is(err, errUpsert) {
		t.Fatalf("error = %v, want upsert failure", err)
	}
	if result.Documents != 2 {
		t.Errorf("acknowledged Documents = %d, want 2", result.Documents)
	}
	if result.Report == nil || result.Report.Status != "failed" {
		t.Errorf("report = %+v, want failed", result.Report)
	}
	if store.calls != 2 || len(store.stored) != 4 {
		t.Errorf("upsert calls = %d, stored = %d, want 2 calls and 4 written records; third batch must not run", store.calls, len(store.stored))
	}
	assertProgress(t, result, 5, 2, 2, 1)
	if got := result.Progress.UncertainBatch; got == nil || *got != (chatgpt.BatchRange{Start: 2, End: 4}) {
		t.Errorf("uncertain batch = %+v, want [2:4]", got)
	}
	if got := result.Report.Failure.Batch; got == nil || *got != (chatgpt.BatchRange{Start: 2, End: 4}) {
		t.Errorf("failure batch = %+v, want [2:4]", got)
	}
}

func assertProgress(t *testing.T, result Result, selected, acknowledged, uncertain, notAttempted int) {
	t.Helper()
	p := result.Progress
	if p.Selected != selected || p.Acknowledged != acknowledged || p.Uncertain != uncertain || p.NotAttempted != notAttempted {
		t.Fatalf("progress = %+v, want selected=%d acknowledged=%d uncertain=%d not_attempted=%d", p, selected, acknowledged, uncertain, notAttempted)
	}
	if p.Selected != p.Acknowledged+p.Uncertain+p.NotAttempted || result.Documents != p.Acknowledged {
		t.Fatalf("progress invariant violated: result=%+v", result)
	}
	if result.Report == nil || result.Report.SchemaVersion != 1 || !reflect.DeepEqual(result.Report.Progress, p) {
		t.Fatalf("report progress does not match result: %+v", result)
	}
}

type scriptedEmbedder struct {
	dim   int
	calls int
	run   func(context.Context, []string, int) ([][]float32, error)
}

func (e *scriptedEmbedder) Dim() int { return e.dim }

func (e *scriptedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	if e.run != nil {
		return e.run(ctx, texts, e.calls)
	}
	return (&embed.FakeEmbedder{DimVal: e.dim}).Embed(ctx, texts)
}

func TestRunRejectsEmbeddingFailuresBeforeUpsert(t *testing.T) {
	embedErr := errors.New("synthetic embedding failure")
	for _, fault := range []string{"embed_error", "too_few", "too_many", "wrong_dimension"} {
		t.Run(fault, func(t *testing.T) {
			store := &progressStore{}
			emb := &scriptedEmbedder{dim: 3, run: func(ctx context.Context, texts []string, call int) ([][]float32, error) {
				vectors, err := (&embed.FakeEmbedder{DimVal: 3}).Embed(ctx, texts)
				if call != 2 {
					return vectors, err
				}
				switch fault {
				case "embed_error":
					return nil, embedErr
				case "too_few":
					vectors = vectors[:1]
				case "too_many":
					vectors = append(vectors, []float32{1, 2, 3})
				case "wrong_dimension":
					vectors[1] = []float32{1, 2}
				}
				return vectors, nil
			}}
			result, err := Run(context.Background(), Options{
				ExportPath: syntheticExport(t, 5), ReportPath: filepath.Join(t.TempDir(), "report.json"),
				Collection: "synthetic", BatchSize: 2,
			}, emb, store)
			if err == nil || result.Report.Status != "failed" || result.Report.Failure.Stage != "embed" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if fault == "embed_error" && !errors.Is(err, embedErr) {
				t.Fatalf("embedding error lost: %v", err)
			}
			assertProgress(t, result, 5, 2, 0, 3)
			if store.calls != 1 || emb.calls != 2 || result.Progress.UncertainBatch != nil || !strings.Contains(err.Error(), "[2:4]") {
				t.Fatalf("unexpected attempts: store=%+v embed_calls=%d err=%v", store, emb.calls, err)
			}
		})
	}
}

func TestRunCancellationBoundaries(t *testing.T) {
	for _, boundary := range []string{"before_store", "before_next_embed", "before_upsert", "after_final_acknowledgement"} {
		t.Run(boundary, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &progressStore{}
			emb := &scriptedEmbedder{dim: 3}
			n, wantAcknowledged, wantCalls := 5, 0, 0
			switch boundary {
			case "before_store":
				cancel()
			case "before_next_embed":
				store.afterUpsert = func(int) { cancel() }
				wantAcknowledged, wantCalls = 2, 1
			case "before_upsert":
				emb.run = func(ctx context.Context, texts []string, call int) ([][]float32, error) {
					vectors, err := (&embed.FakeEmbedder{DimVal: 3}).Embed(ctx, texts)
					if call == 2 {
						cancel()
					}
					return vectors, err
				}
				wantAcknowledged, wantCalls = 2, 1
			case "after_final_acknowledgement":
				n, wantAcknowledged, wantCalls = 2, 2, 1
				store.afterUpsert = func(int) { cancel() }
			}
			result, err := Run(ctx, Options{ExportPath: syntheticExport(t, n), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "synthetic", BatchSize: 2}, emb, store)
			if !errors.Is(err, context.Canceled) || result.Report.Status != "failed" || result.Report.Failure.Stage != "context" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			assertProgress(t, result, n, wantAcknowledged, 0, n-wantAcknowledged)
			if store.calls != wantCalls {
				t.Fatalf("upsert calls=%d want=%d", store.calls, wantCalls)
			}
			if boundary == "before_store" && (store.ensureCalls != 0 || emb.calls != 0) {
				t.Fatalf("called services with canceled context: store=%+v embed_calls=%d", store, emb.calls)
			}
			if boundary == "before_next_embed" && emb.calls != 1 {
				t.Fatalf("embedded the next batch after cancellation: calls=%d", emb.calls)
			}
		})
	}
}

func TestRunReportWriteFailuresKeepProgressAndErrors(t *testing.T) {
	for _, failUpsert := range []bool{false, true} {
		t.Run(fmt.Sprintf("upsert_failure_%t", failUpsert), func(t *testing.T) {
			// Renaming a file onto an existing directory fails after temp creation.
			dir := t.TempDir()
			reportPath := filepath.Join(dir, "previous-report")
			if err := os.Mkdir(reportPath, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(reportPath, "retained")
			if err := os.WriteFile(marker, []byte("old report destination"), 0600); err != nil {
				t.Fatal(err)
			}
			store := &progressStore{}
			if failUpsert {
				store.failAt = 2
			}
			result, err := Run(context.Background(), Options{ExportPath: syntheticExport(t, 5), ReportPath: reportPath, Collection: "synthetic", BatchSize: 2}, &embed.FakeEmbedder{DimVal: 3}, store)
			var linkErr *os.LinkError
			if err == nil || !errors.As(err, &linkErr) || result.Report.Status != "failed" {
				t.Fatalf("report write error lost: result=%+v err=%v", result, err)
			}
			if failUpsert {
				if !errors.Is(err, errUpsert) || result.Report.Failure.Stage != "upsert" {
					t.Fatalf("upsert error lost: %v", err)
				}
				assertProgress(t, result, 5, 2, 2, 1)
			} else {
				assertProgress(t, result, 5, 5, 0, 0)
				if result.Report.Failure.Stage != "write_report" {
					t.Fatalf("failure = %+v", result.Report.Failure)
				}
			}
			if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "old report destination" {
				t.Fatalf("previous destination changed: data=%q err=%v", data, readErr)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 1 {
				t.Fatalf("temporary report leaked: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestRunMaxDocsAndPartialFinalBatch(t *testing.T) {
	for _, maxDocs := range []int{0, 3, 10} {
		t.Run(fmt.Sprintf("max_%d", maxDocs), func(t *testing.T) {
			export := syntheticExport(t, 5)
			exp, err := chatgpt.LoadExport(export)
			if err != nil {
				t.Fatal(err)
			}
			exp.Conversations[0].Mapping["system"] = chatgpt.Message{ID: "system", Message: &chatgpt.MsgBody{Author: chatgpt.Author{Role: "system"}, CreateTime: 6, Content: chatgpt.Content{Parts: []any{"skipped source record"}}}}
			data, err := json.Marshal(exp)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(export, data, 0600); err != nil {
				t.Fatal(err)
			}
			store := &progressStore{}
			reportPath := filepath.Join(t.TempDir(), "report.json")
			result, err := Run(context.Background(), Options{ExportPath: export, ReportPath: reportPath, Collection: "synthetic", BatchSize: 2, MaxDocs: maxDocs}, &embed.FakeEmbedder{DimVal: 3}, store)
			if err != nil || result.Report.Status != "completed" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			selected := 5
			wantSizes := []int{2, 2, 1}
			if maxDocs == 3 {
				selected, wantSizes = 3, []int{2, 1}
			}
			assertProgress(t, result, selected, selected, 0, 0)
			if result.Progress.Classified != 5 || result.Progress.Skipped != 1 || result.Progress.ExcludedByMaxDocs != 5-selected || result.Report.Summary.TotalRecords != 6 || result.Report.Summary.Skipped != 1 || !reflect.DeepEqual(store.batchSizes, wantSizes) {
				t.Fatalf("classification, cap or final batch conflated: progress=%+v summary=%+v sizes=%v", result.Progress, result.Report.Summary, store.batchSizes)
			}
			data, err = os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			var saved chatgpt.IngestReport
			if err := json.Unmarshal(data, &saved); err != nil || !reflect.DeepEqual(saved.Progress, result.Progress) || saved.SchemaVersion != 1 {
				t.Fatalf("persisted report mismatch: saved=%+v err=%v", saved, err)
			}
		})
	}
}

func TestRunDeterministicFullRetryAfterUncertainWrite(t *testing.T) {
	store := &progressStore{failAt: 2}
	opts := Options{ExportPath: syntheticExport(t, 5), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "synthetic", BatchSize: 2}
	first, err := Run(context.Background(), opts, &embed.FakeEmbedder{DimVal: 3}, store)
	if !errors.Is(err, errUpsert) || len(store.stored) != 4 {
		t.Fatalf("first run: result=%+v err=%v store=%+v", first, err, store)
	}
	assertProgress(t, first, 5, 2, 2, 1)
	firstIDs := make(map[any]bool)
	for id := range store.stored {
		firstIDs[id] = true
	}
	store.failAt = 0
	retried, err := Run(context.Background(), opts, &embed.FakeEmbedder{DimVal: 3}, store)
	if err != nil || len(store.stored) != 5 {
		t.Fatalf("retry: result=%+v err=%v store=%+v", retried, err, store)
	}
	assertProgress(t, retried, 5, 5, 0, 0)
	for id := range firstIDs {
		if _, ok := store.stored[id]; !ok {
			t.Fatalf("retry changed prior ID %v", id)
		}
	}
	for _, p := range store.stored {
		if p.ID != PointIDFromDocID(p.Payload["doc_id"].(string)) {
			t.Fatalf("point ID not derived from stable document ID: %+v", p)
		}
	}
}

func TestRunSetupFailureDoesNotAttemptDocuments(t *testing.T) {
	for _, fault := range []string{"ensure", "zero_dimension", "negative_dimension"} {
		t.Run(fault, func(t *testing.T) {
			store := &progressStore{}
			emb := &scriptedEmbedder{dim: 3}
			if fault == "ensure" {
				store.ensureErr = os.ErrPermission
			} else if fault == "zero_dimension" {
				emb.dim = 0
			} else {
				emb.dim = -1
			}
			result, err := Run(context.Background(), Options{ExportPath: syntheticExport(t, 5), ReportPath: filepath.Join(t.TempDir(), "report.json"), Collection: "synthetic", BatchSize: 2}, emb, store)
			if err == nil || result.Report.Status != "failed" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			assertProgress(t, result, 5, 0, 0, 5)
			if store.calls != 0 || emb.calls != 0 || (fault != "ensure" && store.ensureCalls != 0) {
				t.Fatalf("attempted documents on setup failure: store=%+v embed_calls=%d", store, emb.calls)
			}
			if fault == "ensure" && !errors.Is(err, os.ErrPermission) {
				t.Fatalf("ensure error lost: %v", err)
			}
		})
	}
}
