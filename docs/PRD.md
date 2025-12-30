# PRD — Personal Semantic Index & Search Substrate

## 1. Problem Statement

Personal digital data (emails, chats, social archives, notes) is:

* fragmented across formats and platforms
* searchable only lexically or via opaque SaaS tools
* not semantically queryable across sources
* not interoperable with agent/MCP workflows

The user needs a **fast, local, semantic search substrate** that:

* preserves Monocle-level UX clarity and speed
* supports semantic queries *plus* deterministic filters (date, source, keywords)
* is modular, extensible, and MCP-ready
* is usable incrementally, without big upfront cost

---

## 2. Goals & Non-Goals

### Goals

* Semantic search over personal data using embeddings
* Deterministic filtering (date, source, keyword, author)
* Fast, predictable UX (sub-200ms local queries)
* Modular ingestion per data source
* Headless API usable by humans *and* agents
* Incremental development with value unlocked early

### Non-Goals (explicitly out of scope initially)

* Knowledge graphs / ontologies
* Conversational chat UI
* Personalization or recommendation systems
* Multi-user access control
* Real-time ingestion/streaming

---

## 3. Core Use Cases

### Primary Use Cases

1. **Semantic recall**

   * “Find discussions about custody strategy from last year”
2. **Scoped semantic recall**

   * “What did I write about X in ChatGPT vs email?”
3. **Temporal reasoning**

   * “How did my thinking evolve between date A and B?”
4. **Agent/MCP queries**

   * Tool call: `search_documents(query, filters)`

### Secondary Use Cases

* Legal / research auditability
* Long-term personal memory augmentation
* Cross-source synthesis (manual or agent-driven)

---

## 4. First Principles (Design Constraints)

1. **Search quality is determined at ingestion**
2. **Embeddings encode meaning, metadata encodes control**
3. **Precomputation beats intelligence**
4. **Filters precede vectors**
5. **Documents, not files or threads, are the atomic unit**
6. **UX clarity comes from determinism, not AI cleverness**

These principles override convenience choices.

---

## 5. System Architecture (High Level)

```
[Source Exports]
      ↓
[Source Adapters]
      ↓
[Canonical Documents]
      ↓
[Text Normalization + Metadata Enrichment]
      ↓
[Embedding Generation (offline)]
      ↓
[Vector Store + Metadata Index]
      ↓
[Search API]
      ↓
[UX / MCP / CLI]
```

Each stage is replaceable.

---

## 6. Data Model (Canonical Document)

### Document = the unit of search

```ts
Document {
  id: string                 // stable, deterministic
  source: enum               // chatgpt | gmail | instagram | notes | etc
  type: enum                 // message | email | post | comment | doc
  timestamp: unix_ms         // canonical time
  text: string               // normalized semantic content
  embedding: vector          // stored, never recomputed at query time

  metadata: {
    author?: string
    participants?: string[]
    thread_id?: string
    keywords?: string[]
    tags?: string[]
    url?: string
  }
}
```

### Key Design Decisions

* **Threads are relationships**, not documents
* **One embedding per document**
* **Metadata is flat, filterable, and deterministic**
* **IDs are content-stable** (re-ingestion safe)

---

## 7. Embedding Design (Deep)

### What Goes Into the Embedding

* The **canonical text only**
* No boilerplate
* No timestamps
* No UI artifacts
* No metadata concatenation (critical)

**Reason:**
Embeddings should encode *semantic meaning*, not constraints. Mixing metadata degrades similarity space.

### Text Normalization Rules

* Strip signatures, footers, quoted replies (configurable)
* Preserve paragraph structure
* Collapse excessive formatting
* Keep language human-readable

### Embedding Strategy

* Batch/offline embedding
* Version embeddings by model + config
* Treat embeddings as immutable artifacts

---

## 8. Indexing Strategy

### Vector Index

* ANN index (e.g., HNSW)
* Optimized for top-K = 20–50
* Local or single-hop network

### Metadata Index

* Source
* Timestamp (range queries)
* Keywords/tags
* Thread ID (optional)

### Lexical Anchor (Optional but recommended)

* Lightweight keyword index (BM25 / simple match)
* Used for:

  * exact matches
  * boosting
  * auditability

---

## 9. Query Model (Semantic + Deterministic)

### Query Contract

```json
{
  "query": "parental alienation patterns",
  "filters": {
    "source": ["chatgpt", "email"],
    "date": { "from": "2023-01-01", "to": "2024-12-31" },
    "keywords": ["custody"]
  },
  "limit": 20
}
```

### Execution Order (Non-Negotiable)

1. Apply metadata filters
2. Perform vector similarity search
3. Apply optional lexical boost
4. Return ranked, stable results

### Why This Matters

* Predictable latency
* Explainable results
* UX stability
* MCP compatibility

---

## 10. UX Principles (Search-First)

Even if no UI is built initially, UX principles shape the API.

### UX Constraints

* Instant feedback
* Stable ordering
* Visible metadata
* No conversational ambiguity

### UX Emerges From:

* Precomputed embeddings
* Deterministic filters
* Small result sets

Not from AI narration.

---

## 11. Development Phases (Incremental Value)

### Phase 1 — Semantic MVP (2–4 days)

**Deliverables**

* One source adapter (ChatGPT or Gmail)
* Canonical document schema
* Embedding + vector index
* Basic search function

**Acceptance Criteria**

* Semantic search works
* Filters by source and date work
* Results returned < 200ms locally

---

### Phase 2 — Usable Personal Tool (1 week total)

**Deliverables**

* Second data source
* Text normalization improvements
* Keyword extraction
* Basic UI or CLI

**Acceptance Criteria**

* Daily usable
* Hybrid queries work
* No re-ingestion data loss

---

### Phase 3 — Durable Substrate (2 weeks total)

**Deliverables**

* Schema hardening
* Incremental re-indexing
* Search API spec (MCP-ready)
* Performance tuning

**Acceptance Criteria**

* Safe re-runs
* Stable IDs
* Agent-usable API
* Extensible to new sources

---

## 12. Acceptance Criteria (System-Level)

The system is considered **fully functional** when:

* A semantic query with filters returns relevant results consistently
* Results are fast, stable, and explainable
* New sources can be added without refactoring core logic
* Index can be rebuilt without corruption
* Search can be called identically by UI or agent

---

## 13. Risks & Mitigations

| Risk                  | Mitigation               |
| --------------------- | ------------------------ |
| Messy exports         | Adapter isolation        |
| Poor semantic quality | Text normalization first |
| Over-engineering      | Phase gates              |
| Lock-in               | Modular interfaces       |

---

## 14. Final Framing

This is not a “search app”.

It is a **personal semantic substrate**:

* composable
* inspectable
* future-proof
* and intentionally boring at the core

Once this exists, **everything else (UX, agents, graphs)** becomes optional and cheap.
