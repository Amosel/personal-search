# Personal Search

Personal Search lets you search your exported ChatGPT conversations and Google Search history by meaning. Describe what you remember—an idea, a question, or a topic—and retrieve relevant messages or past searches, even when you cannot recall the exact words.

For example, a query such as `how I planned to organize my notes` can help you find an earlier discussion about a filing system. Results include the stored text, timestamp, source, similarity score, and metadata so you can inspect the original context.

The project is a Go application with a browser UI, command-line search, an HTTP API, and an MCP tool for assistants. The default workflow uses local Ollama embeddings and a local Qdrant database; no OpenAI API key is needed. Imports come from export files: refresh the index by importing a newer export when you want to add recent activity.

## What you can search

| Source | Accepted input | What a result represents |
| --- | --- | --- |
| ChatGPT | `conversations.json`, or a ChatGPT export ZIP containing it | One message, with speaker, conversation ID, and title when available |
| Google Search | Google Takeout ZIP containing `Takeout/My Activity/Search/MyActivity.html`, the HTML file itself, or a normalized JSON archive from an external crawler | One Search activity record, with account, original URL when present, and import provenance |

In the browser, select a source and optionally filter by date. ChatGPT also supports speaker and conversation filters, and you can expand a matching message to read the indexed conversation in chronological order. Google results expose their stored metadata and provenance. Date filters include both boundary dates and use UTC days.

ChatGPT and Google Search use separate Qdrant collections. Each search selects one source; results from the two collections are not combined into a shared ranking. The [`chatgpt-conversation-search`](chatgpt-conversation-search) wrapper searches ChatGPT; Google has a separate CLI and is also available through the UI, API, and MCP.

## How it works

1. **Import an archive.** A source adapter extracts text and metadata into documents with stable IDs. ChatGPT ingestion also writes a JSON report showing which records became documents, were skipped, or failed.
2. **Build the index.** An embedding model turns each document's text into a numeric vector. Qdrant stores that vector alongside the text and metadata. Reimporting the same records updates their existing points rather than adding duplicates; records absent from a newer export remain in the index.
3. **Search by meaning.** The server embeds your query, applies the source and metadata filters, and retrieves documents ranked by cosine similarity. The browser, CLIs, and MCP tool all use this HTTP search backend.

Use the same embedding provider, model, and dimensions for importing and searching a collection. Google additionally checks the model-content identity and collection compatibility. When changing models, use a new collection and reimport your archive.

With the default local setup, document text and queries are embedded by Ollama on your machine, and the index stays in Qdrant's persistent local volume. ChatGPT also supports OpenAI embeddings as an optional provider; choosing it sends text and queries to the OpenAI API.

## Get started locally

You need Go **1.23.3 or newer**, Make, `curl`, `jq`, a running Ollama service, and either Apple's `container` CLI or Docker with Compose. Run the following commands from the repository root.

### 1. Prepare the services

Pull the default embedding model into Ollama:

```sh
ollama pull nomic-embed-text:latest
```

Start Qdrant using Apple's container runtime (the Makefile default). Its services must be running first:

```sh
container system start
make qdrant-up
```

For Docker, use this instead:

```sh
QDRANT_RUNTIME=docker make qdrant-up
```

Check that Qdrant, Ollama, and the model are reachable:

```sh
./chatgpt-conversation-search doctor
```

A missing ChatGPT collection is expected before the first import. Qdrant's database dashboard is available at <http://localhost:6333/dashboard>.

### 2. Import your ChatGPT export

```sh
./chatgpt-conversation-search ingest /absolute/path/to/chatgpt-export.zip
# Or pass /absolute/path/to/conversations.json.
```

The wrapper writes `ingest_report.json` in the directory where you invoked it. To choose a report location, use the Make target:

```sh
make chatgpt-ingest EXPORT=/absolute/path/to/conversations.json \
  REPORT_OUT=/absolute/path/to/ingest-report.json
```

### 3. Start the search server

```sh
./chatgpt-conversation-search server
```

Open <http://127.0.0.1:18080/> to search in the browser. Keep the server running while using the UI, search CLIs, or MCP tool.

### 4. Search from another terminal

```sh
./chatgpt-conversation-search search "how I planned to organize my notes"
./chatgpt-conversation-search search --author assistant --limit 5 "filing system"
./chatgpt-conversation-search search --from 2025-01-01 --to 2025-12-31 \
  --format yaml "notes organization"
./chatgpt-conversation-search status
```

The ChatGPT CLI returns JSON by default and also supports YAML. Its filters are `--author`, `--thread_id`, `--from`, and `--to`; `--limit` controls the result count and `--server` selects a different HTTP server.

## Add Google Search history

With Qdrant and Ollama running, validate a Takeout archive before importing it:

```sh
go run ./cmd/ingest_google_search --archive /absolute/path/to/takeout.zip --dry-run
go run ./cmd/ingest_google_search --archive /absolute/path/to/takeout.zip
```

The ZIP importer infers the account from `Takeout/Google Account/*.SubscriberInfo.html` when available. Otherwise, supply `--account` with a stable account namespace. When importing a standalone HTML file, set `--account` explicitly to distinguish accounts. You can also pass a normalized Google Search JSON archive to `--archive`. Use `--max_docs 20` for a bounded first import; `--dry-run` validates the archive without contacting services.

The default Google collection is `google_search`, using local `nomic-embed-text:latest`. The server started above already supports it: choose **Google Search** in the browser, or run:

```sh
go run ./cmd/search_google_search --server http://127.0.0.1:18080 \
  --query "tools for organizing notes" --from 2025-01-01 --to 2025-12-31
```

See [Google Search integration](docs/google-search.md) for archive contracts, model compatibility, custom configuration, and verification status, and [Takeout formats](docs/google-takeout-shapes.md) for supported export shapes.

## Use the API or an assistant

The HTTP server exposes:

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Browser search UI |
| `POST /search` | Semantic search with source and metadata filters |
| `GET /status` | ChatGPT collection name and exact point count; use `?source=google_search` for Google |
| `GET /thread?thread_id=...` | Indexed ChatGPT messages in timestamp order, capped at 5,000 messages with a truncation flag |
| `GET /health` | Qdrant availability check |

For example:

```sh
curl -s http://127.0.0.1:18080/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"organizing notes","filters":{"source":["chatgpt"],"author":"assistant"},"limit":5}'
```

The response has a `results` array; each result contains `id`, `score`, `text`, `timestamp`, `source`, and `metadata`. To search Google, set `filters.source` to `["google_search"]` and omit ChatGPT-only author and conversation filters. Both sources accept `filters.date` with `from` and/or `to` in `YYYY-MM-DD` format.

To expose search to an MCP client, configure it to launch this command from the repository root while the HTTP server is running:

```sh
go run ./cmd/mcp_search --server http://127.0.0.1:18080
```

The stdio server exposes **`search_documents`**, with arguments `query`, `source` (`chatgpt` or `google_search`, defaulting to ChatGPT), `limit`, `from`, `to`, and ChatGPT-only `author` and `thread_id`. `make mcp`, `make chatgpt-mcp`, and `cmd/mcp_chatgpt_search` are existing entrypoints for the same implementation.

## Configuration and indexing limits

The Make targets and wrapper use these defaults:

| Setting | Default |
| --- | --- |
| `QDRANT_RUNTIME` | `container` (`docker` is also supported) |
| `QDRANT_URL` | `http://localhost:6333` |
| `CHATGPT_COLLECTION` | `chatgpt_messages` |
| `CHATGPT_SERVER_ADDR` / `CHATGPT_SERVER_URL` | `127.0.0.1:18080` / `http://127.0.0.1:18080` |
| `CHATGPT_EMBEDDER` | `ollama` |
| `OLLAMA_URL` / `OLLAMA_MODEL` | `http://localhost:11434` / `nomic-embed-text:latest` |
| `CHATGPT_DIM` / `CHATGPT_BATCH` | `0` (infer dimensions) / `8` |
| `CHATGPT_FORMAT` | `json` |

Override settings on a Make command, such as `make chatgpt-server CHATGPT_SERVER_ADDR=127.0.0.1:18081`. Direct `go run` commands have different defaults: the server uses port **8080**, collection **`personal_docs`**, and the **OpenAI** provider. Use explicit flags when running them; OpenAI requires `OPENAI_API_KEY` and a positive `--dim`. Use `--help` to list flags for these Go commands. Google's server settings are separate: `--google_collection`, `--google_ollama_url`, and `--google_ollama_model`.

ChatGPT ingestion extracts and normalizes message text, skipping system messages, empty records, and records without a valid timestamp. It indexes messages individually. For long inputs, Ollama embeds the first 1,500 and last 500 characters, joined by a separator; full normalized text remains stored for retrieval. Search uses vector similarity; keyword filtering and lexical boosting are not implemented.

## Development and further reading

The source adapters live in [`internal/chatgpt`](internal/chatgpt) and [`internal/googlesearch`](internal/googlesearch). Shared document and filter types are in [`internal/model`](internal/model), embedding providers in [`internal/embed`](internal/embed), and database operations in [`internal/qdrant`](internal/qdrant). [`cmd/server`](cmd/server) serves the API and embedded UI; [`internal/searchmcp`](internal/searchmcp) implements the MCP tool.

Run `go test ./...` for the test suite. Integration tests skip when their required services are unavailable. `make smoke`, `make acceptance`, and `make integration` start or reuse Qdrant for live checks; `make semantic-smoke` compares fake and Ollama embeddings.

For the optional real-export parser snapshot check, copy [`.env.local.example`](.env.local.example) to `.env.local`, set `CHATGPT_EXPORT_PATH`, and run `make chatgpt-snapshot`. Only that target sources the local file; it does not configure ingestion. The snapshot has fixed archive-specific counts and a hash, so it is not a general validator for arbitrary exports.

- [ChatGPT workflow and commands](docs/chatgpt_search_mvp.md)
- [ChatGPT source and parsing contract](docs/chatgpt_export_source.md)
- [Google Search integration](docs/google-search.md)
- [Test plan](docs/chatgpt_search_mvp_test_plan.md)
- [Documentation index](docs/README.md)
