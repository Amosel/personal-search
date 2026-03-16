# 1. ChatGPT Adapter Spec

The authoritative specification for ChatGPT export structure, file discovery, and streaming behavior is `docs/chatgpt_export_source.md`.
This document does not redefine source-level behavior.

## 1.1 Source Characteristics (facts)

* Input: ChatGPT **data export** (JSON)
* Structure: conversations → messages
* Messages have:

  * role (`user`, `assistant`, `system`)
  * content blocks (text, sometimes tool metadata)
  * timestamps
* Conversations are **containers**, not semantic units

**Design decision:**
➡️ **Each valid non-system message becomes one Document**
(conversation/thread is metadata only)

---

## 1.2 Adapter Responsibility (strict)

The adapter **must only**:

* Parse the export
* Normalize into canonical `Document`s
* Preserve all recoverable meaning
* Emit deterministic IDs

It **must not**:

* Embed
* Index
* Rank
* Filter

---

## 1.3 Canonical Mapping

### Input → Output Mapping

| ChatGPT Field   | Document Field     | Notes                    |
| --------------- | ------------------ | ------------------------ |
| conversation.id | metadata.thread_id | Stable grouping          |
| message.id      | id                 | Deterministic            |
| role            | metadata.author    | `"user"` / `"assistant"` |
| content.text    | text               | Canonicalized            |
| create_time     | timestamp          | UTC ms                   |
| model           | metadata.tags      | Optional                 |

---

## 1.4 Document ID Strategy (critical)

```text
id = sha256(
  conversation_id +
  "|" +
  message_id
)
```

**Why:**

* Stable across re-exports
* Re-ingestion safe
* No accidental duplication

---

## 1.5 Text Canonicalization Rules (non-negotiable)

### MUST

* Flatten content blocks → plain text
* Preserve paragraph breaks
* Remove:

  * system prompts
  * tool call metadata
  * JSON artifacts
* Normalize whitespace

### MUST NOT

* Summarize
* Truncate
* Merge messages
* Inject metadata into text

**Reason:**
Embeddings must represent *what was said*, not *how it was stored*.

---

## 1.6 Example Output Document

```json
{
  "id": "9c7e…",
  "source": "chatgpt",
  "type": "message",
  "timestamp": 1703028812000,
  "text": "I think the core issue here is that supervision became a substitute for negotiation, not a safety measure.",
  "metadata": {
    "author": "user",
    "thread_id": "conv_abc123",
    "tags": ["gpt-4"]
  }
}
```

---

## 1.7 Adapter Acceptance Criteria

* [ ] Non-system messages with valid timestamp + non-empty canonical text become Documents
* [ ] IDs stable across runs
* [ ] Re-ingestion produces identical output
* [ ] No embeddings generated here

---

# 2. Search API Spec (MCP Tool)

This is the **single primitive** exposed to UI *and* agents.

---

## 2.1 Tool Name

```text
search_documents
```

---

## 2.2 Tool Purpose

> Perform **semantic search over personal indexed documents**, with optional deterministic filters.

No side effects. Idempotent.

---

## 2.3 Tool Input Schema

```json
{
  "query": {
    "type": "string",
    "description": "Semantic query text"
  },
  "filters": {
    "type": "object",
    "optional": true,
    "properties": {
      "source": {
        "type": "array<string>",
        "description": "Data sources to include"
      },
      "date": {
        "type": "object",
        "properties": {
          "from": { "type": "string", "format": "date" },
          "to": { "type": "string", "format": "date" }
        }
      },
      "author": {
        "type": "string",
        "description": "user / assistant / email sender"
      }
    }
  },
  "limit": {
    "type": "number",
    "default": 20
  }
}
```

---

## 2.4 Execution Semantics (must be documented)

**Execution order (fixed):**

1. Apply metadata filters
2. Perform vector similarity search
3. Optional lexical boost
4. Return top-K

**No conversational state.**

---

## 2.5 Tool Output Schema

```json
{
  "results": [
    {
      "id": "string",
      "score": "number",
      "text": "string",
      "timestamp": "string",
      "source": "string",
      "metadata": {
        "author": "string",
        "thread_id": "string",
      }
    }
  ]
}
```

---

## 2.6 Unsupported Filters

`keywords` filtering is not implemented by the current server.
Requests with `filters.keywords` must fail with `400`.

---

## 2.7 Output Guarantees

* Stable ordering for identical inputs
* Scores monotonically decreasing
* Each result independently citable
* No hallucinated content

---

## 2.8 MCP Alignment Notes

* Stateless
* Deterministic
* Chainable
* Agent-safe
* Human-usable

This tool can be:

* called by an LLM agent
* wrapped by a UI
* used in batch analysis
* audited later

---

## 3. System-Level Acceptance Criteria

### ChatGPT Adapter

* [ ] Clean one-message → one-document mapping
* [ ] Stable IDs
* [ ] No semantic loss
* [ ] Zero embedding logic

### Search Tool

* [ ] Semantic queries work
* [ ] Filters work independently
* [ ] Combined queries work
* [ ] <200ms local latency
* [ ] Same contract for humans and agents

---

## 4. Framing (why this is correct)

* Ingestion defines meaning
* Embeddings encode intent
* Metadata encodes control
* Search is declarative, not conversational
* MCP is a **consumer**, not a driver
