package ingest

import (
	"context"
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
	Documents int
	Report    *chatgpt.IngestReport
}

// Run performs the production Personal Search ingestion computation: export
// loading, canonical document selection, embedding, point construction, and
// durable Qdrant upsert. The CLI is only an adapter around this function.
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
	writeFailure := func(stage string, err error) (Result, error) {
		report.MarkFailed(stage, err.Error())
		if opts.ReportPath != "" {
			_ = report.Write(opts.ReportPath)
		}
		return Result{Report: report}, err
	}
	exp, err := chatgpt.LoadExport(opts.ExportPath)
	if err != nil {
		return writeFailure("load_export", fmt.Errorf("load export: %w", err))
	}
	docs, err := chatgpt.ToDocumentsWithReport(exp, report)
	if err != nil {
		return writeFailure("classification", fmt.Errorf("convert export: %w", err))
	}
	if len(docs) == 0 {
		return writeFailure("classification", fmt.Errorf("no documents produced from export"))
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
		points := make([]qdrant.Point, 0, len(batch))
		for k, d := range batch {
			payload := map[string]any{"doc_id": d.ID, "source": d.Source, "type": d.Type, "timestamp_unix_ms": float64(d.Timestamp), "text": d.Text}
			for mk, mv := range d.Metadata {
				payload[mk] = mv
			}
			points = append(points, qdrant.Point{ID: PointIDFromDocID(d.ID), Vector: vecs[k], Payload: payload})
		}
		if err := store.Upsert(ctx, opts.Collection, points); err != nil {
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

func PointIDFromDocID(docID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	return h.Sum64()
}
