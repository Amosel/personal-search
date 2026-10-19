package ingest

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"personal-search/internal/model"

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
	// Documents is the number acknowledged by storage, even on failure.
	Documents int
	Progress  model.IngestProgress
	Report    *chatgpt.IngestReport
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
	progress := model.IngestProgress{}
	var currentBatch *model.BatchRange
	result := func() Result {
		progress.Recount()
		report.Progress = progress
		return Result{Documents: progress.Acknowledged, Progress: progress, Report: report}
	}
	writeFailure := func(stage string, err error) (Result, error) {
		report.MarkFailed(stage, err.Error())
		report.Failure.Batch = currentBatch
		r := result()
		cause := &model.IngestError{Stage: stage, Batch: currentBatch, Progress: progress, Err: err}
		if reportErr := report.Write(opts.ReportPath); reportErr != nil {
			return r, errors.Join(cause, fmt.Errorf("write failure report: %w", reportErr))
		}
		return r, cause
	}
	exp, err := chatgpt.LoadExport(opts.ExportPath)
	if err != nil {
		return writeFailure("load_export", fmt.Errorf("load export: %w", err))
	}
	docs, err := chatgpt.ToDocumentsWithReport(exp, report)
	progress.Classified = report.Summary.DocumentsCreated
	progress.Skipped = report.Summary.Skipped
	if err != nil {
		return writeFailure("classification", fmt.Errorf("convert export: %w", err))
	}
	if len(docs) == 0 {
		return writeFailure("classification", fmt.Errorf("no documents produced from export"))
	}
	if opts.MaxDocs > 0 && len(docs) > opts.MaxDocs {
		docs = docs[:opts.MaxDocs]
	}
	progress.Selected = len(docs)
	progress.Excluded = progress.Classified - progress.Selected
	if err := ctx.Err(); err != nil {
		return writeFailure("canceled", err)
	}
	if emb.Dim() <= 0 {
		return writeFailure("embed", fmt.Errorf("embedding dimension must be positive"))
	}
	if err := store.EnsureCollection(ctx, opts.Collection, emb.Dim()); err != nil {
		return writeFailure("ensure_collection", err)
	}
	for i := 0; i < len(docs); i += opts.BatchSize {
		j := i + opts.BatchSize
		if j > len(docs) {
			j = len(docs)
		}
		currentBatch = &model.BatchRange{Start: i, End: j}
		if err := ctx.Err(); err != nil {
			return writeFailure("canceled", err)
		}
		batch := docs[i:j]
		texts := make([]string, len(batch))
		for k := range batch {
			texts[k] = batch[k].Text
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return writeFailure("embed", fmt.Errorf("embed batch [%d:%d]: %w", i, j, err))
		}
		if err := embed.ValidateVectors(vecs, len(batch), emb.Dim()); err != nil {
			return writeFailure("embed", err)
		}
		points := make([]qdrant.Point, 0, len(batch))
		for k, d := range batch {
			payload := d.Payload()
			points = append(points, qdrant.Point{ID: qdrant.NumericID(PointIDFromDocID(d.ID)), Vector: vecs[k], Payload: payload})
		}
		if err := ctx.Err(); err != nil {
			return writeFailure("canceled", err)
		}
		if err := store.Upsert(ctx, opts.Collection, points); err != nil {
			progress.Uncertain += len(batch)
			progress.UncertainBatches = append(progress.UncertainBatches, *currentBatch)
			return writeFailure("upsert", fmt.Errorf("upsert batch [%d:%d]: %w", i, j, err))
		}
		progress.Acknowledged += len(batch)
	}
	currentBatch = nil
	if err := ctx.Err(); err != nil {
		return writeFailure("canceled", err)
	}
	result()
	report.MarkCompleted()
	if opts.ReportPath != "" {
		if err := report.Write(opts.ReportPath); err != nil {
			report.MarkFailed("write_report", err.Error())
			return result(), &model.IngestError{Stage: "write_report", Progress: progress, Err: err}
		}
	}
	return result(), nil
}

func DefaultReportPath() string {
	return filepath.Join(".", "ingest_report.json")
}

func PointIDFromDocID(docID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	return h.Sum64()
}
