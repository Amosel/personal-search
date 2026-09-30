# Operator entry points

This inventory describes the current ChatGPT search workflow. It assigns a
caller and support status to each path; it does not remove or rename any path.
The ingestion boundary currently accepts ChatGPT exports. Other sources can
add their own adapters while sharing the document, embedding, storage, and
search layers; this inventory does not constrain that design.

## Supported actions and callers

| Action | Recommended operator entry point | Other supported paths |
| --- | --- | --- |
| Check dependencies | `./chatgpt-conversation-search doctor` | `make chatgpt-doctor` |
| Ingest a ChatGPT export | `./chatgpt-conversation-search ingest PATH` | `make chatgpt-ingest EXPORT=PATH`; `go run ./cmd/ingest_chatgpt --export PATH` |
| Check indexed count | `./chatgpt-conversation-search status` | `make chatgpt-status`; HTTP `GET /status` for service callers |
| Start the search service and browser UI | `./chatgpt-conversation-search server` | `make chatgpt-server`; `go run ./cmd/server` for direct configuration |
| Search from a terminal | `./chatgpt-conversation-search search ...` | `make chatgpt-search QUERY=...`; `go run ./cmd/search_chatgpt --query ...` |
| Search from a browser | Open the UI served at the server address | UI calls the service's `/search` and `/thread` endpoints |
| Connect an MCP client | `./chatgpt-conversation-search mcp` | `make chatgpt-mcp`; `go run ./cmd/mcp_chatgpt_search` |

The wrapper is the supported human operator interface. `chatgpt-*` Make
targets are supported for automation and explicit variable configuration.
The direct Go commands are supported lower-level interfaces for callers that
need their CLI flags. HTTP endpoints and the MCP stdio server are supported
integration interfaces. The browser UI is a supported end-user interface.

Support classification for the current paths:

- **Public:** wrapper commands; ChatGPT Make targets; direct
  `cmd/ingest_chatgpt`, `cmd/server`, `cmd/search_chatgpt`, and
  `cmd/mcp_chatgpt_search` commands; HTTP routes; MCP server; browser UI.
- **Compatibility:** no path is designated compatibility-only today. The
  lower-level public paths above remain supported; no removal is implied.
- **Internal/test-only:** `cmd/ingest` and `cmd/analyze` diagnostics, test and
  smoke commands, and package/integration test entry points.
- **Local administration:** Qdrant Make helpers are supported local operator
  utilities, separate from the ChatGPT workflow contract.

### HTTP service contract

The server exposes these caller-facing routes:

- `GET /` and `GET /index.html`: embedded browser UI.
- `GET /health`: Qdrant reachability.
- `GET /status`: configured collection name and exact point count.
- `POST /search`: validated semantic search request; used by the search CLI,
  MCP tool, and browser UI.
- `GET /thread?thread_id=...`: messages for one conversation; used by the UI.

The server listens on loopback by default. The `--addr` flag can change the
listen address; deployments that expose it beyond the local machine must
configure their own network boundary.

### Diagnostic and test paths

`go run ./cmd/ingest PATH` and `go run ./cmd/analyze PATH` are diagnostic
helpers. They report parsing/conversion information and are not the durable
ingestion path. `make smoke`, `make acceptance`, `make integration`,
`make semantic-smoke`, and package tests are verification paths, not operator
APIs. `make qdrant-up`, `make qdrant-down`, `make qdrant-status`, and
`make default-status` are local Qdrant administration helpers; they are not
ChatGPT workflow entry points. No compatibility promise is made for these
diagnostic, test, or local administration paths.

## Configuration defaults and precedence

### Wrapper

- `ingest PATH`: positional path wins over `CHATGPT_EXPORT_PATH`; the
  environment variable is used only when the positional path is omitted.
  Wrapper ingestion always sets `REPORT_OUT` to
  `<directory where wrapper was invoked>/ingest_report.json`.
- `search`: `CHATGPT_FORMAT` defaults to `json`; `--format` overrides it.
  `CHATGPT_SERVER_URL` defaults to `http://127.0.0.1:18080`; `--server`
  overrides it. Search filters (`--author`, `--thread_id`, `--from`, `--to`,
  `--limit`) are passed to the search CLI. Query positional text and `--query`
  text are collected into the query; if both are supplied they are joined.
- `server`: passes through Make configuration, including
  `CHATGPT_SERVER_ADDR`.
- The wrapper changes to the repository directory before running Make. The
  ingest report path is anchored to the original invocation directory; a
  relative export path is therefore resolved from the repository directory.

### Make targets

GNU Make command-line variable assignments override environment variables,
which override the `?=` defaults in `Makefile`. Relevant defaults:

| Variable | Default | Used by |
| --- | --- | --- |
| `QDRANT_URL` | `http://localhost:6333` | Qdrant-dependent targets |
| `CHATGPT_COLLECTION` | `chatgpt_messages` | ChatGPT ingest, server, status, doctor |
| `CHATGPT_SERVER_ADDR` | `127.0.0.1:18080` | Server listen address |
| `CHATGPT_SERVER_URL` | `http://$(CHATGPT_SERVER_ADDR)` | Search and MCP clients |
| `CHATGPT_EMBEDDER` | `ollama` | Ingest and server |
| `OLLAMA_URL` | `http://localhost:11434` | Doctor, ingest, server |
| `OLLAMA_MODEL` | `nomic-embed-text:latest` | Doctor, ingest, server |
| `CHATGPT_DIM` | `0` | Ingest and server; zero enables Ollama dimension inference |
| `CHATGPT_BATCH` | `8` | Ingest |
| `CHATGPT_FORMAT` | `json` | Search output |
| `REPORT_OUT` | `ingest_report.json` | Ingest report path |

`CHATGPT_SERVER_URL` may be set directly; otherwise Make derives it from
`CHATGPT_SERVER_ADDR`. Wrapper `search` passes its resolved format and server
URL as Make command-line variables, taking precedence over Make environment
and defaults. Wrapper `ingest` similarly passes the resolved export as
`EXPORT` and fixes `REPORT_OUT` to the invocation directory. Direct Make
ingest requires `EXPORT`; `CHATGPT_EXPORT_PATH` is not a Make default.

`CHATGPT_SEARCH_ARGS` has an empty Make default and carries filter flags to
`cmd/search_chatgpt`. Prefer wrapper flags or direct Make variables over
setting this string manually.

### Direct Go commands

Go CLI flag values override the defaults declared by each command. These
defaults differ from the ChatGPT Make workflow in some cases:

- `cmd/ingest_chatgpt` and `cmd/server`: `--qdrant` defaults to
  `http://localhost:6333`, `--collection` to `personal_docs`, and
  `--embedder` to `openai`. Other flags and defaults are listed by `--help`.
- `cmd/search_chatgpt` and `cmd/mcp_chatgpt_search`: server defaults to
  `http://127.0.0.1:8080`, distinct from the Make workflow's port `18080`.
- `OPENAI_API_KEY` supplies the default for `--openai_key` in ingest and
  server; an explicit `--openai_key` overrides it. This key is needed only
  with the OpenAI embedder. Make's ingest/server recipes inherit this
  environment variable; Make defines no separate key variable or flag.
- `CHATGPT_EXPORT_PATH`, `CHATGPT_FORMAT`, `CHATGPT_SERVER_URL`, `QDRANT_URL`,
  `OLLAMA_URL`, and `OLLAMA_MODEL` are not read as configuration by the direct
  Go commands. Pass their corresponding flags instead. `CHATGPT_EXPORT_PATH`
  is also used by an optional snapshot test.
- `cmd/ingest_chatgpt --report-out` overrides its default report path
  `./ingest_report.json`, resolved from the process working directory.

## Source of truth

This page records observed behavior in `chatgpt-conversation-search`,
`Makefile`, `cmd/`, `internal/`, and `cmd/server/web/`. Update this inventory
when a caller path, default, or precedence rule changes. Compatibility and
retirement policy is tracked separately under GitHub issue #2.
