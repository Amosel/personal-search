---
name: chatgpt-conversation-search
description: Search a user's locally indexed ChatGPT conversation history through the `search_documents` MCP tool. Use when the user asks what they previously discussed in ChatGPT, wants recall from prior prompts/responses, or wants date/thread/author-scoped history search.
---

# ChatGPT Conversation Search

Use this skill when the user wants retrieval from their prior ChatGPT conversations that have been exported, ingested, and exposed through the local MCP tool.

## Preconditions

- local Qdrant is running
- ChatGPT export has been ingested into the configured local collection
- local MCP server `chatgpt-search-mcp` is active
- default local path uses Ollama embeddings; no OpenAI key needed
- helper wrapper available in repo root: `./chatgpt-conversation-search`

## Tool

Use MCP tool:

```text
search_documents
```

The tool is scoped to ChatGPT conversations by the server-side wrapper.

## Parameters

- `query` required
- `limit` optional
- `author` optional
- `thread_id` optional
- `from` optional, `YYYY-MM-DD`
- `to` optional, `YYYY-MM-DD`

Do not send `keywords`; current backend rejects them.

## Usage Rules

1. Start with the user's own phrasing as `query`.
2. Add `author` only if they ask for user-only or assistant-only messages.
3. Add `thread_id` only when the user references a specific thread.
4. Add `from` and `to` only when the user asks for date scoping.
5. If no results, broaden filters before concluding nothing is present.

## Response Style

- quote or summarize only returned results
- distinguish between no matches and no indexed data
- include timestamps/thread context when relevant

## Failure Modes

- no local index data
- MCP server unavailable
- unsupported filter request, especially `keywords`
