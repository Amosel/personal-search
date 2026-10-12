# Google Search integration

Input: normalized JSON archive produced by the existing external crawler, or raw Google Takeout ZIP/HTML containing `Takeout/My Activity/Search/MyActivity.html`. This repository does not acquire Google history or modify that crawler. No dependencies added. Raw export shape notes: [google-takeout-shapes.md](google-takeout-shapes.md).

## Import

```sh
go run ./cmd/ingest_google_search --archive /path/google-search.json --dry-run
go run ./cmd/ingest_google_search --archive /path/takeout.zip --dry-run
# First import bounded to 20 records:
go run ./cmd/ingest_google_search --archive /path/google-search.json --max_docs 20
# After verifying retrieval and repeat import, complete the archive:
go run ./cmd/ingest_google_search --archive /path/google-search.json
```

Defaults: local Qdrant localhost:6333, local Ollama localhost:11434, model nomic-embed-text:latest, dedicated collection google_search, batch 8. Override with --qdrant, --ollama_url, --ollama_model, --collection, --batch. `--account` supplies the account namespace for raw HTML; raw ZIP imports infer it from `Takeout/Google Account/*.SubscriberInfo.html` when present. --max_docs 0 imports all; dry-run validates the entire archive without service calls. Service startup and model installation are separate operator actions; neither happens automatically.

Archive contract: `{schema_version:1, source:"google_search", documents:[Document], report:{...}}`. Document source google_search, type search_query, positive Unix-millisecond timestamp, query text, canonical ID. Required metadata: account namespace, original URL field (may be empty if the original had none), raw record, nonempty provenance array with locator, 64-hex SHA-256, nonnegative integer record_index. Reserved payload keys are rejected. JSON numeric values in raw records retain their representation.

IDs are verified against the crawler's SHA-256 of compact UTF-8 JSON `["google_search", account, timestamp_unix_ms, exact_query]`. Account, time or query changes produce distinct events. Points use a deterministic 128-bit UUID derived from the canonical ID. Reimport upserts the same points; counts printed are acknowledged records, **not newly created points**. Missing events are retained. A matching event's payload/provenance is replaced by the current archive; retain original and normalized archives independently. No deletion or full-history completeness claim.

Each point includes the Ollama model-content digest and embedding preprocessing version. Before importing or querying Google, every existing collection payload is checked for matching identity/source, and vector dimension and Cosine distance are checked. Unmarked legacy collections, mixed sources and incompatible models fail closed: use a new dedicated collection; never delete the old one. Ollama tag contents are checked before/after embedding. Concurrent model changes/imports are unsupported. Full compatibility scans add query latency proportional to collection size.

Upserts require wait=true and status=completed. On failure, acknowledged counts include only prior confirmed batches; an unconfirmed batch may have reached Qdrant. Errors identify the batch range, zero-based, exclusive upper bound. Rerun safely with the same archive/model. CLI exits nonzero on failure; model initialization failures report zero acknowledged. Validation failures happen before indexing.

## UI, HTTP and MCP

Run the existing server using the existing ChatGPT configuration. For example, if ChatGPT was indexed with local nomic:

```sh
go run ./cmd/server --collection personal_docs --embedder ollama --ollama_model nomic-embed-text:latest
# Same server; Google source is selected explicitly:
go run ./cmd/search_google_search --query "topic" --from 2025-01-01 --to 2025-12-31
go run ./cmd/mcp_search --server http://127.0.0.1:8080
```

Server --collection/--embedder/--ollama_model retain their ChatGPT meaning and defaults. Google routes separately using --google_collection (google_search), --google_ollama_url (localhost:11434), --google_ollama_model (nomic-embed-text:latest), regardless of the ChatGPT provider. Google collection must differ from ChatGPT's. Querying does not create the Google collection; import it first. Existing ChatGPT embedding/model conventions remain unchanged; the new compatibility enforcement applies to Google.

Browser source selector: ChatGPT (default) or Google Search. Date bounds inclusive UTC days; reversed/invalid dates rejected. Speaker/conversation controls disabled for Google. Google cards show account, original URL and expandable stored metadata/provenance. Original URLs render as links only for HTTP(S), without altering the stored URL. Results retain Unix-millisecond timestamps in metadata and RFC3339 timestamps with fractional precision. ChatGPT conversation expansion remains available and source-scoped.

HTTP `/search`: `filters.source:["google_search"]` selects Google, `filters.date:{from:"YYYY-MM-DD",to:"YYYY-MM-DD"}` filters dates. Google with another source, speaker or conversation filter is rejected. Omitted source preserves existing collection routing. Cross-collection ranking is unsupported. `/status?source=google_search` returns the Google collection count; unfiltered `/status` retains ChatGPT behavior.

Canonical MCP executable: `cmd/mcp_search`; `cmd/mcp_chatgpt_search` remains a compatibility alias sharing `internal/searchmcp`. Tool `search_documents` retained. New argument `source:"chatgpt"|"google_search"`; omitted source defaults to ChatGPT. `from`/`to` inclusive UTC dates apply to either source. `author`/`thread_id` remain ChatGPT-only. Structured results include complete stored metadata/provenance.

## Verification status (2026-10-07)

Mocked/unit checks cover archive validation, canonical IDs/account separation, repeat-import IDs, acknowledged partial counts, HTTP collection/source/date routing, millisecond timestamps and provenance, MCP source/date forwarding, same-dimension model rejection, unmarked/mixed collection rejection, compatibility pagination and upsert completion status. These are not live Qdrant results.

Live local Ollama: 768-dimensional embeddings, model digest 0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f.

Qdrant started using existing Docker Compose service, without image pulling, collection deletion or infrastructure edits. Existing chatgpt_messages count: 61,704 before/after verification, untouched. Synthetic live test created and retained collection google_search_synthetic_verify_1791394495699469000: **3 unique points**, 3 acknowledged on first import, 3 acknowledged on repeat import, final count unchanged. Verified known-record top retrieval through HTTP routing, inclusive date inclusion/exclusion, source exclusion, millisecond timestamp/URL/account/provenance preservation, and same-dimension model-mismatch rejection. These are actual Ollama/Qdrant operations on synthetic fixtures, **not actual-history import**.

Opt-in repeatable live test: `GOOGLE_SEARCH_LIVE=1 go test ./cmd/server -run '^TestGoogleLiveSynthetic$' -v`. Each run creates and retains its own synthetic collection; never deletes collections. Default tests skip it.

Actual-history indexed this run: **0**. User is waiting for Takeout; full import and actual-history known-record retrieval deferred until delivery. Browser JavaScript syntax checked, visual/browser interaction unverified. MCP source/date forwarding tested with mocks; both generic and compatibility executables exercised for initialization/tool listing with identical responses. Live MCP searches against actual-history data not exercised. No full-history semantic relevance claim.
