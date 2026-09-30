# Source adapter contract

`internal/ingest` accepts a `SourceAdapter` explicitly. The shared ingest package does not select or import a source parser.

```go
type SourceAdapter interface {
	LoadDocuments(path string) ([]model.Document, ingestreport.Report, error)
}
```

## Adapter responsibilities

- Read and interpret its source input.
- Classify each source record as a document, skip, or failure; keep source-specific identifiers and reasons in the adapter-owned report.
- Return only valid canonical `model.Document` values. Call `Document.Validate()` before returning them.
- Preserve stable source identity in `Document.ID`, `Document.Source`, and source-native provenance in `Document.Metadata`.
- Return a report on success and on source-processing failure when one can be created.
- Keep ordering and IDs deterministic for the same input.

The source adapter owns its report schema. `ingestreport.Report` exposes only the shared lifecycle operations: mark failure/completion, write, report state, and common record counts. It does not define record identifiers or classification reasons. ChatGPT keeps its current per-message outcomes, reasons, native identifiers, and JSON report fields in `chatgpt.IngestReport`.

## Shared ingest responsibilities

`ingest.Run` applies the document cap, embeds document text, and calls the storage interface with canonical documents and corresponding vectors. It does not parse source data or build backend-specific points. The Qdrant `DocumentStore` maps canonical documents to Qdrant point IDs, vectors, and payloads.

Source selection stays with the caller. `cmd/ingest_chatgpt` selects `chatgpt.SourceAdapter`; another source can provide its own adapter and report without adding a parser dependency to `internal/ingest`.

## Failure behavior

Adapter errors stop ingestion before collection creation or embedding. If the adapter returned a report, ingestion marks it failed and writes it to the configured report path. Storage and embedding failures also mark the source-owned report failed with the failing stage.
