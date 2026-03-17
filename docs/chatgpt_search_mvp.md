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
```

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
