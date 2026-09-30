# ChatGPT Export Source Specification

## Authority

This document describes the ChatGPT export source interface as implemented.

This is the canonical reference for what the source accepts, reads, and guarantees.
The shared multi-source boundary is defined in [source_adapter_contract.md](source_adapter_contract.md).

---

## Input Contract

### Accepted Input

The source MUST accept a single file path pointing to either:
- A ChatGPT export JSON file (`conversations.json`)
- A ChatGPT export ZIP archive (containing `conversations.json`)

The source MUST NOT accept:
- Directory paths
- Multiple files
- URLs or remote resources

### File Format

The source MUST handle two JSON formats:

**Format 1: Object wrapper**
```json
{
  "conversations": [...]
}
```

**Format 2: Array**
```json
[...]
```

The source MUST attempt to parse as array format first, then fall back to object format.

If both formats fail, the source MUST return an error.

---

## Reading Behavior

### File Loading

The source MUST:
- Open the input path (JSON) or ZIP entry (`conversations.json`)
- Decode JSON using `json.Decoder` (streaming decode)
- Return an error if the file does not exist
- Return an error if the file is not readable
- Return an error if JSON parsing fails

The source MUST NOT:
- Attempt recovery from malformed JSON
- Silently skip unparseable content

### Conversation Processing

For each conversation in the export, the source MUST:
- Read all messages from the `mapping` field
- Extract message metadata (ID, create_time, author, content)

The source MUST skip:
- Messages where `message` field is null
- Messages where `author.role == "system"`

The source MUST NOT:
- Filter by date, author, or any other criteria at read time
- Merge or split conversations
- Rewrite or interpret conversation structure

---

## Ordering Guarantees

### Message Ordering

Within each conversation, messages MUST be ordered by:
1. Primary: `create_time` (ascending)
2. Tiebreaker: `id` (lexicographic ascending)

This sort MUST be stable across multiple invocations of the same export.

The source MUST NOT:
- Preserve original mapping iteration order
- Group by author or thread
- Reorder based on content

### Conversation Ordering

Conversations MUST be processed in the order they appear in the JSON array.

The source MUST NOT reorder conversations.

---

## Error/Skip Rules

### The source MUST return an error and halt if:

- The file does not exist
- The file cannot be read
- JSON is malformed
- Document validation fails for any message

### The source skips individual messages (continues processing) when:

- `message` field is null
- `author.role == "system"`
- Canonicalized text is empty
- `create_time <= 0` (after conversion to unix ms)

### The source MUST NOT:

- Log warnings and proceed
- Provide partial results on error
- Attempt fallback or recovery strategies

---

## Text Canonicalization

Before text is used, the source MUST:
- Normalize line endings (CRLF → LF)
- Trim leading/trailing whitespace
- Collapse excessive blank lines (max 1 consecutive)
- Extract text from `content.parts` or `content.text`

If canonicalized text is empty (whitespace-only), the source skips that message.

The source MUST NOT:
- Summarize, truncate, or rewrite text
- Inject metadata into text
- Translate or normalize language
- Remove paragraph structure

---

## Document Construction

For each valid message, the source MUST construct a Document with:

**Required fields:**
- `id`: SHA256(conversation_id | message_id)
- `source`: `"chatgpt"` (hardcoded)
- `type`: `"message"` (hardcoded)
- `timestamp`: `create_time * 1000` (unix milliseconds)
- `text`: canonicalized content

**Metadata:**
- `author`: message author role (`"user"` | `"assistant"`)
- `thread_id`: conversation ID
- `title`: conversation title (if present)
- `created`: ISO8601 timestamp (if timestamp > 0)

The source MUST call `Document.Validate()` on each constructed document.

If validation fails, the source MUST return an error.

---

## What This Source Does NOT Do

The source does NOT:
- Accept directory paths
- Handle multiple exports simultaneously
- Merge duplicate messages
- Filter by date, author, or content
- Preserve thread relationships beyond metadata
- Generate embeddings
- Write to storage
- Provide progress callbacks
- Support cancellation or timeouts
- Validate conversation structure
- Enforce conversation consistency

---

## Memory Characteristics

The source:
- Streams ZIP entries (if ZIP input)
- Uses bounded buffering for JSON decoding
- Memory usage bounded by largest conversation, not file size
- Constructs documents incrementally
- Returns all documents as a single slice

ZIP-level streaming is supported. JSON-level is bounded by conversation size.

---

## Determinism Guarantee

Given the same export file, the source MUST produce:
- Identical document IDs
- Identical text (after canonicalization)
- Identical ordering
- Identical metadata

Across multiple invocations, the source MUST be deterministic.

---

## Error Reporting

Errors SHOULD include:
- The underlying cause
- Conversation and message IDs when available

Errors SHOULD NOT include message content (privacy).

---

## Non-Functional Constraints

### Performance

The source makes NO performance guarantees.

The source does NOT optimize for:
- Large exports (>10GB)
- Low-memory environments
- Concurrent processing

### Compatibility

The source supports ONLY the observed ChatGPT export formats as of implementation.

The source does NOT guarantee forward compatibility with future export formats.

If the format changes, the source MUST fail explicitly rather than silently misparse.

---

## Known Export Format Issues

### Timestamp Location (December 2024)

**Issue:** OpenAI's ChatGPT export places `create_time` in the message body (`message.message.create_time`), not at the message node level (`message.create_time`).

**Evidence:**
- Analysis of 1,475 conversations shows consistent pattern
- ~3 messages per conversation have `null` timestamps (structural nodes: root, system, context)
- Actual conversation messages have valid timestamps in nested location

**Workaround Applied:**
- `export.go`: `MsgBody` struct includes `CreateTime` field
- `adapter.go`: Reads from `m.Message.CreateTime` instead of `m.CreateTime`
- Marked with `// WORKAROUND:` comments for easy removal

**When to Remove:**
If OpenAI fixes the export format to place `create_time` at the message node level, remove:
1. `CreateTime` field from `MsgBody` struct
2. All `// WORKAROUND:` comment blocks in adapter.go
3. This documentation section

**References:**
- `docs/chatgpt_conversations_json_format.md#critical-observation-create_time-field`
- [Community discussion](https://community.openai.com/t/questions-about-the-json-structures-in-the-exported-conversations-json/954762)

---

## Source Defect Taxonomy

The following defects may be encountered in ChatGPT export data. Each has a defined detection rule, severity, and handling strategy.

| Defect Code | Detection Rule | Severity | Action |
|------------|---------------|----------|--------|
| `INVALID_TIMESTAMP` | `create_time` ≤ 0, null, or missing | `skip` | Skip message, continue processing |
| `EMPTY_TEXT_AFTER_CANON` | Canonicalized text is empty string | `skip` | Skip message, continue processing |
| `MALFORMED_MESSAGE_NODE` | `message` field is null | `skip` | Skip message, continue processing |
| `UNORDERED_CREATE_TIME` | `create_time` < previous message in conversation | `coerce` | Sort enforces ordering |
| `MISSING_CONVERSATION_ID` | Conversation has no `id` field | `fatal` | Halt ingestion with error |

**Severity Levels:**
- `fatal`: Halt ingestion immediately, return error
- `skip`: Skip this message, log defect, continue processing
- `coerce`: Apply deterministic heuristic, continue processing

---

## Decoder Heuristics Table

For defects marked `coerce`, the following deterministic heuristics are applied:

| Defect Code | Heuristic Rule | Determinism |
|------------|---------------|-------------|
| `UNORDERED_CREATE_TIME` | Enforce sort by `(create_time, id)` | Pure function, stable sort |

**Heuristic Requirements:**
- MUST be pure functions (no external state)
- MUST be deterministic (same input → same output)
- MUST NOT depend on processing order
- MUST NOT require lookahead or lookbehind beyond current conversation

**No Heuristics For:**
Defects marked `skip` or `fatal` have no heuristics. They are handled by skipping or halting.
