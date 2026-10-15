package ingest

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"

	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
)

// Store is the small application boundary used by ingestion.  Keeping this
// interface here lets tests exercise the real export conversion and embedding
// path without requiring a running Qdrant service.
type Store interface {
	EnsureCollection(context.Context, string, int) error
	Upsert(context.Context, string, []qdrant.Point) error
}

type Options struct {
	ExportPath string
	ReportPath string
	Collection string
	BatchSize  int
	MaxDocs    int
}

type Result struct {
	// Documents is the count acknowledged in this run, not newly created points
	// or a guarantee of disk durability. Failed upserts do not count here.
	Documents int
	Report    *chatgpt.IngestReport
	Progress  chatgpt.IngestProgress
}

// Run performs the production Personal Search ingestion computation: export
// loading, canonical document selection, embedding, point construction, and
// acknowledged Qdrant upsert. The CLI is only an adapter around this function.
func Run(ctx context.Context, opts Options, emb embed.Embedder, store Store) (Result, error) {
	if opts.ExportPath == "" || opts.Collection == "" || opts.ReportPath == "" {
		return Result{}, fmt.Errorf("export path, collection, and report path are required")
	}
	if opts.BatchSize <= 0 {
		return Result{}, fmt.Errorf("batch size must be > 0")
	}
	if opts.MaxDocs < 0 {
		return Result{}, fmt.Errorf("max docs must be >= 0")
	}
	if emb == nil || store == nil {
		return Result{}, fmt.Errorf("embedder and store are required")
	}
	report := chatgpt.NewIngestReport(opts.ExportPath)
	result := func() Result {
		return Result{Documents: report.Progress.Acknowledged, Report: report, Progress: report.Progress}
	}
	writeFailure := func(stage string, batch *chatgpt.BatchRange, err error) (Result, error) {
		res := result()
		report.MarkFailed(stage, err.Error())
		report.Failure.Batch = batch
		if writeErr := report.Write(opts.ReportPath); writeErr != nil {
			err = errors.Join(err, fmt.Errorf("write failure report: %w", writeErr))
		}
		return res, err
	}
	exp, err := chatgpt.LoadExport(opts.ExportPath)
	if err != nil {
		return writeFailure("load_export", nil, fmt.Errorf("load export: %w", err))
	}
	docs, err := chatgpt.ToDocumentsWithReport(exp, report)
	if err != nil {
		return writeFailure("classification", nil, fmt.Errorf("convert export: %w", err))
	}
	if len(docs) == 0 {
		return writeFailure("classification", nil, fmt.Errorf("no documents produced from export"))
	}
	if opts.MaxDocs > 0 && len(docs) > opts.MaxDocs {
		report.Progress.ExcludedByMaxDocs = len(docs) - opts.MaxDocs
		docs = docs[:opts.MaxDocs]
	}
	report.Progress.Selected = len(docs)
	report.Progress.NotAttempted = len(docs)
	if err := ctx.Err(); err != nil {
		return writeFailure("context", nil, fmt.Errorf("before ensure collection: %w", err))
	}
	dim := emb.Dim()
	if dim <= 0 {
		return writeFailure("embed", nil, fmt.Errorf("embedding dimension must be > 0, got %d", dim))
	}
	if err := store.EnsureCollection(ctx, opts.Collection, dim); err != nil {
		return writeFailure("ensure_collection", nil, err)
	}
	for i := 0; i < len(docs); i += opts.BatchSize {
		j := i + opts.BatchSize
		if j > len(docs) {
			j = len(docs)
		}
		batch := docs[i:j]
		batchRange := &chatgpt.BatchRange{Start: i, End: j}
		if err := ctx.Err(); err != nil {
			return writeFailure("context", batchRange, fmt.Errorf("before embed batch [%d:%d]: %w", i, j, err))
		}
		texts := make([]string, len(batch))
		for k := range batch {
			texts[k] = batch[k].Text
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return writeFailure("embed", batchRange, fmt.Errorf("embed batch [%d:%d]: %w", i, j, err))
		}
		if len(vecs) != len(batch) {
			return writeFailure("embed", batchRange, fmt.Errorf("embed batch [%d:%d]: vector count got=%d want=%d", i, j, len(vecs), len(batch)))
		}
		for k, vec := range vecs {
			if len(vec) != dim {
				return writeFailure("embed", batchRange, fmt.Errorf("embed batch [%d:%d]: vector %d dimension got=%d want=%d", i, j, k, len(vec), dim))
			}
		}
		points := make([]qdrant.Point, 0, len(batch))
		for k, d := range batch {
			payload := map[string]any{"doc_id": d.ID, "source": d.Source, "type": d.Type, "timestamp_unix_ms": float64(d.Timestamp), "text": d.Text}
			for mk, mv := range d.Metadata {
				payload[mk] = mv
			}
			points = append(points, qdrant.Point{ID: PointIDFromDocID(d.ID), Vector: vecs[k], Payload: payload})
		}
		if err := ctx.Err(); err != nil {
			return writeFailure("context", batchRange, fmt.Errorf("before upsert batch [%d:%d]: %w", i, j, err))
		}
		if err := store.Upsert(ctx, opts.Collection, points); err != nil {
			report.Progress.Uncertain = len(batch)
			report.Progress.UncertainBatch = batchRange
			report.Progress.NotAttempted -= len(batch)
			return writeFailure("upsert", batchRange, fmt.Errorf("upsert batch [%d:%d]: completion unknown; may have been applied: %w", i, j, err))
		}
		report.Progress.Acknowledged += len(batch)
		report.Progress.NotAttempted -= len(batch)
	}
	if err := ctx.Err(); err != nil {
		return writeFailure("context", nil, fmt.Errorf("after acknowledged upserts: %w", err))
	}
	res := result()
	report.MarkCompleted()
	if err := report.Write(opts.ReportPath); err != nil {
		report.MarkFailed("write_report", err.Error())
		return res, fmt.Errorf("write completed report: %w", err)
	}
	return res, nil
}

func DefaultReportPath() string {
	return filepath.Join(".", "ingest_report.json")
}

func PointIDFromDocID(docID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	return h.Sum64()
}
