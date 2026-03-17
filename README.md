# 1. System Overview (as implemented)

## Pipeline

- ChatGPT export file path (`.json` or `.zip`) -> `chatgpt.LoadExport` -> `chatgpt.ToDocuments` -> embed text via selected embedder (`openai`, `fake`, `ollama`) -> create/ensure Qdrant collection -> upsert points with payload -> HTTP search server embeds query -> Qdrant vector search with optional metadata filter -> returns JSON search results

## Entrypoints (CLI / server)

- Wrapper CLI: [chatgpt-conversation-search](/Users/amoselmaliah/dev/projects/personal-search/chatgpt-conversation-search)
- Ingest CLI: [cmd/ingest_chatgpt/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/ingest_chatgpt/main.go)
- Search server: [cmd/server/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/server/main.go)
- Search CLI: [cmd/search_chatgpt/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/search_chatgpt/main.go)
- MCP server: [cmd/mcp_chatgpt_search/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/mcp_chatgpt_search/main.go)
- Diagnostic CLIs:
  - [cmd/ingest/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/ingest/main.go)
  - [cmd/analyze/main.go](/Users/amoselmaliah/dev/projects/personal-search/cmd/analyze/main.go)

# 2. CLI Commands (actual)

## `./chatgpt-conversation-search doctor`

- Arguments/flags: none
- Behavior: runs `make chatgpt-doctor`; checks Qdrant reachable, Ollama reachable, configured Ollama model present, and prints collection count if collection exists

## `./chatgpt-conversation-search ingest /path/to/conversations.json`

- Arguments/flags: positional export path, or `CHATGPT_EXPORT_PATH`
- Behavior: runs `make chatgpt-ingest EXPORT=...`

## `./chatgpt-conversation-search status`

- Arguments/flags: none
- Behavior: runs `make chatgpt-status`; prints exact point count for configured collection or errors if missing

## `./chatgpt-conversation-search server`

- Arguments/flags: none
- Behavior: runs `make chatgpt-server`

## `./chatgpt-conversation-search search [--author ...] [--thread_id ...] [--from ...] [--to ...] [--limit ...] [--format ...] [--server ...] "query text"`

- Arguments/flags:
  - query text positional or via `--query`
  - `--author`
  - `--thread_id`
  - `--from`
  - `--to`
  - `--limit`
  - `--format`
  - `--server`
  - env `CHATGPT_FORMAT`, `CHATGPT_SERVER_URL`
- Behavior: forwards to `make chatgpt-search`, which runs `cmd/search_chatgpt`; always scopes request to ChatGPT source via shared request builder

## `./chatgpt-conversation-search mcp`

- Arguments/flags: none
- Behavior: runs `make chatgpt-mcp`

## `go run ./cmd/ingest_chatgpt`

- Flags:
  - `--export`
  - `--qdrant` default `http://localhost:6333`
  - `--collection` default `personal_docs`
  - `--embedder` default `openai`
  - `--openai_key`
  - `--model` default `text-embedding-3-large`
  - `--ollama_url` default `http://localhost:11434`
  - `--ollama_model` default `nomic-embed-text:latest`
  - `--dim` default `0`
  - `--batch` default `64`
  - `--max_docs` default `0`
- Behavior: loads export, converts to documents, optionally truncates docs by `max_docs`, ensures Qdrant collection with embedder dimension, embeds in batches, upserts all points, prints progress

## `go run ./cmd/server`

- Flags:
  - `--addr` default `:8080`
  - `--qdrant`
  - `--collection`
  - `--embedder`
  - `--openai_key`
  - `--model`
  - `--ollama_url`
  - `--ollama_model`
  - `--dim`
- Behavior: builds embedder, ensures collection exists, starts HTTP server with `/health` and `/search`

## `go run ./cmd/search_chatgpt`

- Flags:
  - `--query` required
  - `--server` default `http://127.0.0.1:8080`
  - `--limit` default `10`
  - `--format` default `json`
  - `--author`
  - `--thread_id`
  - `--from`
  - `--to`
- Behavior: builds ChatGPT-scoped search request, sends POST `/search`, emits JSON or YAML

## `go run ./cmd/mcp_chatgpt_search`

- Flags:
  - `--server` default `http://127.0.0.1:8080`
- Behavior: stdio JSON-RPC/MCP-like server exposing `search_documents`

## `go run ./cmd/ingest <path>`

- Behavior: diagnostic helper; loads export, converts to docs, prints counts and determinism hash

## `go run ./cmd/analyze <path>`

- Behavior: diagnostic helper; prints export analysis, skip counts, role/timestamp distribution, sample document

# 3. API Endpoints (actual)

## `GET /health`

- Method: `GET`
- Request shape: none
- Response shape:
  - `200`: `{"status":"ok"}`
  - `503`: plain text `"qdrant unavailable"`
  - `405`: plain text `"method not allowed"`
- Behavior: checks `qc.Healthy`

## `POST /search`

- Method: `POST`
- Request shape:

```json
{
  "query": "string",
  "filters": {
    "source": ["string"],
    "date": {"from":"YYYY-MM-DD","to":"YYYY-MM-DD"},
    "keywords": ["string"],
    "author": "string",
    "thread_id": "string"
  },
  "limit": 10
}
```

- Response shape:

```json
{
  "results": [
    {
      "id": "string",
      "score": 0.0,
      "text": "string",
      "timestamp": "RFC3339 UTC string",
      "source": "string",
      "metadata": { "...": "payload fields" }
    }
  ]
}
```

- Behavior:
  - decodes JSON
  - validates request
  - defaults `limit` to `20` if `<= 0`
  - embeds query text
  - calls Qdrant search with `with_payload=true`
  - maps Qdrant payload to response fields
  - errors:
    - `400` invalid JSON
    - `400` validation failure
    - `500` embedding error
    - `500` search error

# 4. Data Model / Storage

## Structures used for stored records

- Canonical document structure: [internal/model/document.go](/Users/amoselmaliah/dev/projects/personal-search/internal/model/document.go)
  - `id string`
  - `source string`
  - `type string`
  - `timestamp int64` JSON key `timestamp_unix_ms`
  - `text string`
  - `metadata map[string]any`
  - `keywords []string`
- Search request/result structures: [internal/model/filters.go](/Users/amoselmaliah/dev/projects/personal-search/internal/model/filters.go)
  - `Filters`
    - `source []string`
    - `date *DateRange`
    - `keywords []string`
    - `author string`
    - `thread_id string`
  - `SearchRequest`
    - `query`
    - `filters`
    - `limit`
  - `SearchResult`
    - `id`
    - `score`
    - `text`
    - `timestamp`
    - `source`
    - `metadata`

## Indexing backend

- Qdrant via [internal/qdrant/client.go](/Users/amoselmaliah/dev/projects/personal-search/internal/qdrant/client.go)
- Collection ensured with vector `size=<dim>` and `distance="Cosine"`

## Payload shape sent to vector DB

- Base payload:
  - `doc_id`
  - `source`
  - `type`
  - `timestamp_unix_ms`
  - `text`
- Plus all document metadata fields, currently from ChatGPT adapter:
  - `author`
  - `thread_id`
  - optional `title`
  - optional `created`

## Point ID

- FNV-1a 64-bit hash of `doc.ID` for Qdrant point ID in ingest CLI

## ChatGPT document ID

- SHA256 of `conversationID + "|" + messageID`

# 5. Ingest Behavior (current, as-is)

## How records are parsed

- `LoadExport` accepts:
  - JSON file
  - ZIP containing `conversations.json`
- Handles top-level JSON:
  - array of conversations
  - object with `conversations` field
- In object format, unknown top-level fields are decoded and discarded

## Record conversion

- For each conversation:
  - iterates `mapping`
  - skips entries where `message == nil`
  - skips entries where `author.role == "system"`
  - sorts remaining messages by `create_time`, then `id`
  - extracts text from `content.parts` first, else `content.text`
  - canonicalizes text:
    - CRLF -> LF
    - trim each line
    - keep at most one consecutive blank line
    - trim final result
  - skips message if canonicalized text is empty
  - converts `create_time * 1000` to unix ms
  - skips if timestamp `<= 0`
  - constructs `Document`
  - validates `Document`; any validation failure returns error and halts

## What happens on invalid records

- Malformed JSON/file open failure halts
- Invalid document after construction halts

## Whether records are skipped, normalized, or fail

- Empty text/system/null message/invalid timestamp are skipped, not errors
- `max_docs > 0` truncates document slice after conversion, before embedding

## Implicit behavior

- Embedder dimension for Ollama:
  - if `--dim=0`, code probes `/api/embed` with `"dimension probe"` and uses returned vector length
- Ollama embedder clips long text before embedding:
  - max 2000 runes
  - keeps head/tail with `\n...\n`
  - stored payload text remains full canonical text

# 6. Search Behavior (current)

## Semantic search implementation

- Query text embedded via selected embedder
- Qdrant vector search at `/collections/<collection>/points/search`
- Cosine distance configured at collection creation

## Filters implemented (exact list)

- `source`
- `author`
- `thread_id`
- `date.from`
- `date.to`

## Unsupported filter

- `keywords`
- validation returns error `"keywords filter is not supported"`

## How filters are applied

- `source` -> Qdrant `should` clauses on payload key `source`
- `author` -> Qdrant `must` match on payload key `author`
- `thread_id` -> Qdrant `must` match on payload key `thread_id`
- `date.from/to` -> Qdrant range on `timestamp_unix_ms`
  - `from` parsed at UTC day start
  - `to` parsed and expanded to day end `23:59:59.999`

## Ranking behavior

- Qdrant returns scored points
- client sorts results descending by score before returning

## ChatGPT-scoped search path

- `internal/chatgptsearch.BuildRequest` always injects `filters.source=["chatgpt"]`
- used by search CLI and MCP server

# 7. Tests

## Acceptance tests

- [internal/integration/acceptance_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/acceptance_test.go)
- Validates:
  - ingest idempotency by point count after re-ingest
  - `/health`
  - happy-path `/search`
  - required result fields/metadata
  - empty query -> `400`
  - invalid JSON -> `400`
  - invalid date -> `400`
  - unsupported keywords -> `400`
  - top-1 determinism across repeated identical query
  - author filter
  - thread filter
  - date filter
  - source no-match
  - source OR behavior
  - empty collection returns empty results

## Integration tests

- [internal/integration/chatgpt_ingest_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/chatgpt_ingest_search_test.go)
  - ingest then server search basic flow
- [internal/integration/e2e_cli_chatgpt_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/e2e_cli_chatgpt_search_test.go)
  - CLI search against server
- [internal/integration/e2e_cli_ingest_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/e2e_cli_ingest_search_test.go)
  - CLI ingest then search
- [internal/integration/e2e_mcp_chatgpt_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/e2e_mcp_chatgpt_search_test.go)
  - MCP initialize + tools/call path
- [internal/integration/e2e_server_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/e2e_server_search_test.go)
  - HTTP server search path
- [internal/integration/e2e_ollama_chatgpt_search_test.go](/Users/amoselmaliah/dev/projects/personal-search/internal/integration/e2e_ollama_chatgpt_search_test.go)
  - Ollama ingest with `--dim 0`
  - Ollama-backed HTTP search
  - Ollama-backed search CLI
  - Ollama-backed MCP

## Unit tests

- Request builder scopes to ChatGPT and preserves filters
- HTTP client handles success and non-200
- Search CLI requires query, emits JSON/YAML
- MCP handler initialize/list/call/unknown tool
- Filters reject keywords
- Embedder tests:
  - OpenAI request/dim handling
  - Ollama config validation
  - Ollama request/response
  - Ollama dimension mismatch
  - Ollama dimension detection
  - Ollama clipping

# 8. Tooling / Runtime

## Docker

- [docker-compose.yml](/Users/amoselmaliah/dev/projects/personal-search/docker-compose.yml)
- Service: `qdrant`
- Image: `qdrant/qdrant:latest`
- Ports: `6333`, `6334`
- Volume: `qdrant_data:/qdrant/storage`

## Make commands

- [Makefile](/Users/amoselmaliah/dev/projects/personal-search/Makefile)
- `qdrant-up`
- `qdrant-down`
- `qdrant-status`
- `default-status`
- `chatgpt-doctor`
- `chatgpt-ingest`
- `chatgpt-server`
- `chatgpt-search`
- `chatgpt-mcp`
- `chatgpt-status`
- `smoke`
- `acceptance`
- `integration`
- `semantic-smoke`

## Make defaults

- `QDRANT_URL=http://localhost:6333`
- `CHATGPT_COLLECTION=chatgpt_messages`
- `CHATGPT_SERVER_ADDR=127.0.0.1:18080`
- `CHATGPT_SERVER_URL=http://127.0.0.1:18080`
- `CHATGPT_EMBEDDER=ollama`
- `OLLAMA_URL=http://localhost:11434`
- `OLLAMA_MODEL=nomic-embed-text:latest`
- `CHATGPT_DIM=0`
- `CHATGPT_BATCH=8`
- `CHATGPT_FORMAT=json`
- `CHATGPT_SEARCH_ARGS=` empty by default

## Environment setup

- Qdrant local default at `http://localhost:6333`
- Ollama local default at `http://localhost:11434`
- OpenAI key used only when `--embedder openai`

## How to run ingest

- `make chatgpt-ingest EXPORT=/absolute/path/to/conversations.json`
- Or wrapper `./chatgpt-conversation-search ingest /absolute/path/to/conversations.json`

## How to run search

- Start server: `make chatgpt-server`
- Search: `make chatgpt-search QUERY="..." [CHATGPT_FORMAT=yaml]`
- Or wrapper `./chatgpt-conversation-search search ...`

## MCP runtime

- `make chatgpt-mcp`
- Stdio JSON-RPC server exposing tool `search_documents`

## Skill/runtime artifacts

- Source skill exists: [skills/chatgpt-conversation-search/SKILL.md](/Users/amoselmaliah/dev/projects/personal-search/skills/chatgpt-conversation-search/SKILL.md)
- Codebase includes no in-repo automatic MCP registration logic beyond the MCP server binary and skill file

# 9. Known Gaps (only if explicitly present in code/tests)

- `keywords` filter rejected explicitly; not implemented
- Lexical boost not implemented in current search server; docs note it as not implemented
- Wrapper `search` supports only:
  - `--author`
  - `--thread_id`
  - `--from`
  - `--to`
  - `--limit`
  - `--format`
  - `--server`
  - any other `--*` flag errors as unsupported
- `cmd/ingest` and `cmd/analyze` are explicitly marked diagnostic-only in code
- Live ChatGPT account sync: UNKNOWN
- Additional source adapters beyond ChatGPT in production path: UNKNOWN
