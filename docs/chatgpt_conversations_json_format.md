# ChatGPT Export conversations.json Format Specification

## Authority

This document describes the observed structure of the `conversations.json` file as exported by ChatGPT in December 2024.

Generated from empirical analysis of 1,475 conversations using genson-rs schema inference.

---

## File Structure

### Top Level

The file is a **JSON array** containing conversation objects:

```json
[
  { /* conversation 1 */ },
  { /* conversation 2 */ },
  ...
]
```

### Conversation Object

Each conversation has the following structure:

```typescript
{
  // Required fields
  "id": string,                    // UUID format
  "conversation_id": string,       // Duplicate of id
  "title": string,                 // Human-readable title
  "create_time": number,           // Unix timestamp (seconds.microseconds)
  "update_time": number,           // Unix timestamp (seconds.microseconds)
  "mapping": object,               // Map of message_id -> MessageNode
  "current_node": string,          // ID of the current active message
  "default_model_slug": string,    // e.g., "gpt-4o", "gpt-4"

  // Optional/nullable fields
  "is_archived": boolean,
  "gizmo_id": string | null,
  "gizmo_type": string | null,
  "conversation_template_id": string | null,
  "conversation_origin": string | null,
  "is_starred": boolean | null,
  "async_status": number | null,
  "disabled_tool_ids": string[]
}
```

**Key Observations:**
- `id` and `conversation_id` are always identical
- `create_time` and `update_time` are present at conversation level
- `mapping` is the core container for all messages

---

## Message Node Structure

The `mapping` field is an object where:
- **Keys**: Message UUIDs
- **Values**: Message node objects

### Message Node Schema

```typescript
{
  "id": string,                    // UUID, matches the mapping key
  "parent": string | null,         // Parent message ID (null for root)
  "children": string[],            // Array of child message IDs
  "message": MessageBody | null    // The actual message content (null for root nodes)
}
```

**Tree Structure:**
- Messages form a directed acyclic graph (DAG)
- Root node has `parent: null`
- Leaf nodes have `children: []`
- Branching occurs when multiple responses exist for same prompt

---

## Message Body Structure

When `message` is non-null, it contains:

```typescript
{
  // Core fields
  "id": string,                    // UUID
  "author": {
    "role": string,                // "user" | "assistant" | "system" | "tool"
    "name": string | null,         // Tool name if role=tool
    "metadata": object
  },
  "content": Content,              // See Content section below
  "create_time": number | null,    // Unix timestamp (seconds.microseconds)
  "update_time": number | null,

  // Status fields
  "status": string,                // "finished_successfully", etc.
  "recipient": string,             // "all"
  "channel": string | null,
  "end_turn": boolean | null,
  "weight": number,                // Seems to be 0.0 or 1.0

  // Metadata
  "metadata": object               // Highly variable structure
}
```

### Author Roles Observed

| Role | Description | create_time Pattern |
|------|-------------|---------------------|
| `user` | User messages | Usually non-null |
| `assistant` | AI responses | Usually non-null |
| `system` | System messages | Usually null |
| `tool` | Tool execution results | Usually non-null |

---

## Content Structure

The `content` field varies by `content_type`:

### Text Content (`content_type: "text"`)

```typescript
{
  "content_type": "text",
  "parts": string[]                // Array of text strings
}
```

### Multimodal Text (`content_type: "multimodal_text"`)

```typescript
{
  "content_type": "multimodal_text",
  "parts": Array<string | ImageAssetPointer>
}
```

Where `ImageAssetPointer`:
```typescript
{
  "content_type": "image_asset_pointer",
  "asset_pointer": string,         // e.g., "file-service://file-XXXXX"
  "size_bytes": number,
  "width": number,
  "height": number,
  "fovea": number | null,
  "metadata": object
}
```

### Code Content (`content_type: "code"`)

```typescript
{
  "content_type": "code",
  "language": string,              // "python", "javascript", etc.
  "text": string,                  // The code
  "response_format_name": string | null
}
```

### User Context (`content_type: "user_editable_context"`)

```typescript
{
  "content_type": "user_editable_context",
  "user_profile": string,
  "user_instructions": string
}
```

### Tool Results (`content_type: "tether_browsing_display"`)

```typescript
{
  "content_type": "tether_browsing_display",
  "result": string,
  "summary": string | null,
  "assets": array | null,
  "tether_id": string | null
}
```

---

## Metadata Field Variations

The `message.metadata` field is highly variable. Common patterns:

### User Message Metadata
```typescript
{
  "request_id": string,
  "timestamp_": "absolute",
  "message_type": null,
  "message_source": null,
  "attachments"?: Attachment[],
  "serialization_metadata"?: object
}
```

### Assistant Message Metadata
```typescript
{
  "model_slug": string,            // "gpt-4o", "gpt-4", etc.
  "default_model_slug": string,
  "parent_id": string,
  "request_id": string,
  "timestamp_": "absolute",
  "message_type": null,
  "finish_details"?: {
    "type": string,                // "stop", "max_tokens"
    "stop_tokens"?: number[]
  },
  "is_complete"?: boolean,
  "citations"?: Citation[],
  "content_references"?: Reference[]
}
```

### Tool Message Metadata
```typescript
{
  "command": string,               // "spinner", "context_stuff", etc.
  "status": string,
  "model_slug": string,
  "default_model_slug": string,
  "parent_id": string,
  "request_id": string,
  "timestamp_": "absolute",
  "is_visually_hidden_from_conversation"?: boolean
}
```

---

## Critical Observation: create_time Field

**Location Ambiguity:**
- The `create_time` field exists at **TWO levels**:
  1. `conversation.create_time` (conversation level) - always present
  2. `message.create_time` (message body level) - **frequently null**

**Empirical Finding:**
- Out of 23,769 messages analyzed:
  - **20,324 messages (85%)** have `message.create_time = null`
  - Only **3,445 messages (15%)** have valid timestamps

**Breakdown by role:**
- `user`: 7,840 messages
- `assistant`: 10,229 messages
- `tool`: 2,255 messages
- `system`: 1,970 messages (excluded from count above)

**Impact on Ingestion:**
- Previous decoder coerced null timestamps to 1ms
- Current decoder skips messages with `create_time ≤ 0`
- **Result**: 100% data loss (0 documents created from 18,474 previously valid messages)

---

## Observed Issues & Defects

### 1. Null create_time Epidemic
- **85% of messages** lack valid `create_time`
- Unclear why ChatGPT export omits this field
- Conversation-level `create_time`/`update_time` exist but don't help per-message ordering

### 2. Tree Structure Complexity
- Messages form DAG, not simple list
- Multiple response branches possible
- Current decoder flattens by iterating `mapping` keys (loses tree structure)

### 3. Inconsistent Content Types
- 11+ distinct `content_type` values observed
- No enumeration or schema published by OpenAI
- Tool messages have nested content structures

### 4. Metadata Explosion
- Over 50 distinct metadata field names observed
- No consistent structure across message types
- Contains ephemeral data (request_ids, model_slugs, UI state)

---

## Schema Generation Metadata

- **Tool**: genson-rs 0.2.0
- **Sample Size**: 1,000 conversations (of 1,475 total)
- **Total Messages**: ~16,000 analyzed
- **Schema Output**: `/tmp/chatgpt_schema.json`
- **Generation Date**: 2024-12-30

---

## Recommendations

1. **Timestamp Recovery Strategy Needed**
   - Consider using conversation-level timestamps as fallback
   - Infer message order from tree structure (parent/children)
   - Use message ID lexicographic order as last resort

2. **Content Type Whitelist**
   - Define which content types to ingest
   - Skip tool/system messages explicitly
   - Extract text from multimodal parts

3. **Tree Traversal**
   - Implement proper DAG traversal
   - Follow parent → children links
   - Preserve conversation flow

4. **Metadata Pruning**
   - Extract only stable, meaningful fields
   - Ignore ephemeral request_ids, UI state
   - Focus on: model_slug, finish_details, citations
