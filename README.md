# Personal Search

Local semantic search over exported personal data.

Current polished path:
- ChatGPT export ingest
- local Qdrant index
- local Ollama embeddings
- HTTP search server
- CLI search
- MCP tool `search_documents`

## Quick Start

```bash
make qdrant-up
./chatgpt-conversation-search doctor
./chatgpt-conversation-search ingest /absolute/path/to/conversations.json
./chatgpt-conversation-search server
CHATGPT_FORMAT=yaml ./chatgpt-conversation-search search "custody strategy"
./chatgpt-conversation-search search --author assistant --limit 3 "custody strategy"
```

Operator detail:
- [docs/chatgpt_search_mvp.md](/Users/amoselmaliah/dev/projects/personal-search/docs/chatgpt_search_mvp.md)
- [docs/README.md](/Users/amoselmaliah/dev/projects/personal-search/docs/README.md)

## Main Entry Points

- wrapper: `./chatgpt-conversation-search`
- ingest CLI: `./cmd/ingest_chatgpt`
- search server: `./cmd/server`
- search CLI: `./cmd/search_chatgpt`
- MCP server: `./cmd/mcp_chatgpt_search`

## Repo Layout

- `cmd/`: binaries
- `internal/chatgpt/`: export parsing + canonical document adapter
- `internal/embed/`: embedding providers and factory
- `internal/qdrant/`: vector store client
- `internal/chatgptsearch/`: ChatGPT-scoped request/client wrapper
- `internal/integration/`: end-to-end and acceptance coverage
- `skills/`: Codex skill source
- `docs/`: specs and MVP/operator docs
