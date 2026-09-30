package ingest

import (
	"context"
	"fmt"
	"path/filepath"

	"personal-search/internal/embed"
	"personal-search/internal/ingestreport"
	"personal-search/internal/model"
)

// SourceAdapter converts one source input into validated canonical Documents.
// Source-specific parsing and classification belong in the adapter; ingestion
// only embeds and stores the resulting Documents.
type SourceAdapter interface {
	LoadDocuments(path string) ([]model.Document, ingestreport.Report, error)
}

// Store is the source-neutral storage boundary used by ingestion. Keeping this
// interface here lets tests exercise source conversion and embedding without a
// running storage service.
type Store interface {
	EnsureCollection(context.Context, string, int) error
	Upsert(context.Context, string, []model.Document, [][]float32) error
}

type Options struct {
	InputPath  string
	ReportPath string
	Collection string
	BatchSize  int
	MaxDocs    int
}

type Result struct {
	Documents int
	Report    ingestreport.Report
}

// Run ingests canonical documents from the supplied source adapter.
func Run(ctx context.Context, opts Options, source SourceAdapter, emb embed.Embedder, store Store) (Result, error) {
	if opts.InputPath == "" || opts.Collection == "" || opts.ReportPath == "" {
		return Result{}, fmt.Errorf("input path, collection, and report path are required")
	}
	if opts.BatchSize <= 0 {
		return Result{}, fmt.Errorf("batch size must be > 0")
	}
	if opts.MaxDocs < 0 {
		return Result{}, fmt.Errorf("max docs must be >= 0")
	}
	if source == nil || emb == nil || store == nil {
		return Result{}, fmt.Errorf("source adapter, embedder, and store are required")
	}
	var report ingestreport.Report
	writeFailure := func(stage string, err error) (Result, error) {
		if report != nil {
			report.MarkFailed(stage, err.Error())
			_ = report.Write(opts.ReportPath)
		}
		return Result{Report: report}, err
	}
	docs, report, err := source.LoadDocuments(opts.InputPath)
	if err != nil {
		return writeFailure("source", fmt.Errorf("load source documents: %w", err))
	}
	if report == nil {
		return Result{}, fmt.Errorf("source adapter returned a nil report")
	}
	if len(docs) == 0 {
		return writeFailure("source", fmt.Errorf("source produced no documents"))
	}
	if opts.MaxDocs > 0 && len(docs) > opts.MaxDocs {
		docs = docs[:opts.MaxDocs]
	}
	if err := store.EnsureCollection(ctx, opts.Collection, emb.Dim()); err != nil {
		return writeFailure("ensure_collection", err)
	}
	for i := 0; i < len(docs); i += opts.BatchSize {
		j := i + opts.BatchSize
		if j > len(docs) {
			j = len(docs)
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
		if err := store.Upsert(ctx, opts.Collection, batch, vecs); err != nil {
			return writeFailure("upsert", fmt.Errorf("upsert batch [%d:%d]: %w", i, j, err))
		}
	}
	report.MarkCompleted()
	if opts.ReportPath != "" {
		if err := report.Write(opts.ReportPath); err != nil {
			return Result{Report: report}, err
		}
	}
	return Result{Documents: len(docs), Report: report}, nil
}

func DefaultReportPath() string {
	return filepath.Join(".", "ingest_report.json")
}
