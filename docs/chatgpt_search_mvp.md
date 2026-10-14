# ChatGPT Search MVP

## Purpose

Search exported ChatGPT conversations locally through:
- a dedicated ingest command
- the existing HTTP search backend
- a ChatGPT-scoped CLI
- a ChatGPT-scoped MCP tool

## Defaults

- Qdrant URL: `http://localhost:6333`
- collection: `chatgpt_messages`
- server address: `127.0.0.1:18080`
- embedder: `ollama`
- Ollama URL: `http://localhost:11434`
- Ollama model: `nomic-embed-text:latest`
- Ollama dimension: auto-inferred when `--dim=0`

No OpenAI key required for the default path.

## Doctor

```bash
make chatgpt-doctor
```

## Commands

Ingest:

```bash
make chatgpt-ingest EXPORT=/absolute/path/to/chatgpt-export.zip
./chatgpt-conversation-search ingest /absolute/path/to/conversations.json
go run ./cmd/ingest_chatgpt --export /absolute/path/to/conversations.json --report-out /absolute/path/to/ingest-report.json
```

The ingest CLI and Make target write `./ingest_report.json` relative to their working directory by default. The wrapper writes it in the directory where you invoked the wrapper. Set `--report-out` or `REPORT_OUT` to choose another writable destination.

Reports use `schema_version: 1` and retain the existing classification `summary`
and `entries`. The separate `progress` fields describe this run:

| Field | Meaning |
| --- | --- |
| `classified` | Canonical documents created during source classification |
| `selected` | Documents chosen after `--max_docs` |
| `acknowledged` | Documents in batches whose upsert returned success |
| `uncertain` | Documents in an upsert batch that returned an error; it may have been applied |
| `not_attempted` | Selected documents never sent to upsert, including embedding failures |
| `skipped` | Source records skipped during classification |
| `excluded_by_max_docs` | Classified documents excluded by the import cap, separate from source skips |

`selected = acknowledged + uncertain + not_attempted`. `Result.Documents` is
the acknowledged count for this run, including on failure; it is not the number
of new unique points or a guarantee of disk durability. An `uncertain_batch`
and a batch-scoped `failure.batch` use zero-based selected-document indexes
with an exclusive end. Embedding count/dimension errors stop before upsert.
Cancellation before an upsert leaves the batch not attempted; cancellation
or another error returned by an invoked upsert leaves it uncertain. A successful
upsert is counted before checking subsequent cancellation.

The CLI prints classification and ingestion progress on failure and exits
nonzero. Reports are written privately (0600) via a same-directory temporary
file, file sync, close and atomic rename. If report writing fails, the returned
report is failed and retains acknowledged progress; operation and report-write
errors are both inspectable. A previous report on disk may remain and does not
describe the failed current write. Older unversioned reports remain readable,
but their historical acknowledged counts cannot be reconstructed.

After a partial failure, retry the full unchanged export against the same
collection with the same embedding model. Stable IDs update the same logical
records, including any uncertain batch that was applied. This is a full retry,
not automatic resume or transactional rollback; no compensating deletes occur.

Run server:

```bash
make chatgpt-server
./chatgpt-conversation-search server
```

Search from terminal:

```bash
make chatgpt-search QUERY="custody strategy"
./chatgpt-conversation-search search "custody strategy"
CHATGPT_FORMAT=yaml ./chatgpt-conversation-search search "custody strategy"
./chatgpt-conversation-search search --author assistant --limit 3 "custody strategy"
```

Run MCP server:

```bash
make chatgpt-mcp
./chatgpt-conversation-search mcp
```

Check collection status:

```bash
make chatgpt-status
./chatgpt-conversation-search status
```

## Supported Search Parameters

- `query`
- `limit`
- `author`
- `thread_id`
- `from`
- `to`

## Unsupported

- `keywords`

Requests with `keywords` fail explicitly.
