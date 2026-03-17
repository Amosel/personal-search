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

## Commands

Ingest:

```bash
make chatgpt-ingest EXPORT=/absolute/path/to/chatgpt-export.zip
```

Run server:

```bash
make chatgpt-server
```

Search from terminal:

```bash
make chatgpt-search QUERY="custody strategy"
```

Run MCP server:

```bash
make chatgpt-mcp
```

Check collection status:

```bash
make chatgpt-status
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
