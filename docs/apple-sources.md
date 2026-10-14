# Apple Notes, Voice Memos, and merged search

Both ingest targets use Python 3's standard library for read-only source extraction and the existing Go/Ollama/Qdrant stack for indexing. No new package dependency. Defaults: local `nomic-embed-text:latest`, separate `apple_notes` and `voicememo` collections. Embedding model digest, dimensions, distance, and source are checked before import/search.

## Apple Notes

Source project: [sweetrb/apple-notes-mcp](https://github.com/sweetrb/apple-notes-mcp). Use its `export-notes-json` tool, saving `structuredContent` unchanged into one JSON file per page. Start at offset 0, follow `page.nextOffset` until `page.hasMore` is false, and retain the same `modifiedSince` throughout. One export snapshot per directory; keep source exports outside git. Avoid library edits during paging: the upstream offset protocol is not a frozen snapshot.

```sh
go run ./cmd/ingest_apple_notes --input /absolute/path/to/notes-pages --dry-run
go run ./cmd/ingest_apple_notes --input /absolute/path/to/notes-pages
```

A single complete JSON export is also accepted. Input shape is `accounts[].folders[].notes[]`; each note includes `id`, `plaintext`, `created`, and `modified`. The upstream [type definitions](https://github.com/sweetrb/apple-notes-mcp/blob/main/src/types.ts) and [export tool](https://github.com/sweetrb/apple-notes-mcp/blob/main/src/index.ts) were inspected on 2026-10-14. Legacy exports without page metadata are accepted, but cannot establish export completeness.

Missing/overlapping pages, inconsistent totals, duplicate note IDs, invalid timestamps and missing IDs fail validation before any index writes. Locked, content-omitted and empty notes are counted separately and skipped. Images/attachments are not searched. Text uses the source's plaintext; dates use note modification time. Creation time, title, account, folder, native ID, export hash, record hash and file locator remain in metadata.

The reusable workflow is `~/dev/scripts/skills/apple-notes-ingest/SKILL.md`. Creating its repo source does not activate it in Codex; runtime rollout is separate.

## Voice Memos

Use the existing `$voicememo` skill for source refresh and transcription. Run its doctor before live store access; then use its documented ingest/transcribe commands with explicit bounds. Apple transcripts are preferred; Whisper fallback requires an already installed engine/model. Personal Search never transcribes or modifies audio itself.

```sh
go run ./cmd/ingest_voicememos \
  --input "$HOME/Library/Mobile Documents/com~apple~CloudDocs/CodeData/voicememo/voicememo_robot.db" \
  --dry-run
# Remove --dry-run to index cached transcripts.
```

The importer opens SQLite with `mode=ro`, uses the latest transcript per memo, and indexes only nonempty successful transcripts without `needs_refresh`. It counts missing, stale, empty and failed/missing-engine transcripts separately. Recording start uses the robot DB's `created_at`, never an exported audio file's filesystem date. Native recording ID, memo/transcript IDs, transcript engine/version/time, audio path and duration remain attached to every result. The cached robot DB does not prove live-store completeness.

SQLite WAL access can require permission to open sidecars even for a read-only connection. Report an access failure; do not bypass it with `immutable=1`, which can ignore committed WAL content.

## Import semantics

Both commands support `--max_docs`, `--batch`, `--collection`, `--qdrant`, `--ollama_url`, and `--ollama_model`. `--dry-run` extracts and validates all input without contacting embedding/index services. Reports distinguish total candidates, selected and acknowledged records. Qdrant upserts may partially succeed before an error; stable IDs make retry safe.

Native identity determines document IDs, so rename/body edits update existing points. Imports are upsert-only: absent, deleted, newly locked, or stale source records do **not** remove previously indexed content. Keep source exports/DB versions when historical reconstruction matters; this index stores the latest imported text, not version history. Long texts use the existing first-1,500/last-500-character embedding policy, with full text retained; middle-only concepts can be missed.

## Serving and merged search

The existing server adds `--notes_collection`, `--voice_collection`, `--apple_ollama_url`, and `--apple_ollama_model` (environment counterparts: `NOTES_COLLECTION`, `VOICE_COLLECTION`, `APPLE_OLLAMA_URL`, `APPLE_OLLAMA_MODEL`). Defaults match the importers. Existing ChatGPT/Google settings remain separate.

Choose Apple Notes, Voice Memos, or All sources in the browser. MCP `search_documents` accepts `source: "apple_notes"`, `"voicememo"`, or `"all"`. HTTP accepts explicit subsets:

```sh
curl -s http://127.0.0.1:18080/search -H 'Content-Type: application/json' \
  -d '{"query":"kitchen plans","filters":{"source":["apple_notes","voicememo"]},"limit":10}'
```

Each collection uses its own compatible query embedder. Merged results receive `1/(60 + source_rank)` rather than comparing raw cosine scores from potentially different embedding spaces. Ties preserve requested source order (All: ChatGPT, Google, Notes, Voice). This interleaves ranked lists fairly; it does not calibrate cross-source semantic relevance or deduplicate equivalent content across sources. Raw similarity remains in `metadata.source_score`; `ranking` is `reciprocal_rank_k60`. Single-source searches retain cosine scores. Limits apply globally, maximum 1,000.

All requested sources must be available. An unavailable/missing collection or embedding mismatch fails the entire request with the source named. Use an explicit subset if some sources have not been imported. Omitted source preserves the existing ChatGPT default. Legacy arbitrary source filters (for example `email`) still query the primary collection; the MCP lists only the four named sources and `all`. Author/conversation filters require ChatGPT alone; date filters apply to every requested source. `/status?source=all` sums counts, also requiring all sources.

## Verification

```sh
python3 -B -m unittest discover -s internal/applearchive -p '*_test.py'
go test ./internal/applearchive ./cmd/server ./internal/searchmcp ./internal/qdrant
```

Fixtures exercise Notes paging/skip/error handling, stable IDs, timestamp handling, read-only VoiceMemo selection, and HTTP merged routing/ranking/failure semantics. Fixture checks do not establish live Notes export coverage or personal semantic retrieval quality.
