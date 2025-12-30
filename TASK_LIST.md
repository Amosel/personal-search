# Personal Search - Task Breakdown & Dependency Map

## Principles
- **Orthogonal Tasks**: Each task owns one concern, zero overlap
- **Contract-First**: Interfaces defined before implementations
- **Fail-Fast**: Invalid input → immediate panic/error, no silent degradation
- **Simple Tests**: Boundary validation only, no internal logic testing
- **Pipeline Architecture**: Data flows one direction through strict contracts

---

## Task Dependency Graph

```
LAYER 0 (Infrastructure)
├─ T1: Docker/Qdrant Setup
└─ T2: Go Module Init

LAYER 1 (Core Contracts)
├─ T3: Canonical Document Model ◄─── ALL DOWNSTREAM TASKS DEPEND ON THIS
├─ T4: Embedder Interface
└─ T5: Filter Model

LAYER 2 (Adapters - Parallel)
├─ T6: Document ID Generator (depends: T3)
├─ T7: Text Canonicalizer (depends: T3)
├─ T8: ChatGPT Export Parser (depends: none)
└─ T9: ChatGPT Adapter (depends: T3, T6, T7, T8)

LAYER 3 (Embedding - Parallel)
└─ T10: OpenAI Embedder (depends: T4)

LAYER 4 (Storage - Parallel)
├─ T11: Qdrant REST Client (depends: none)
├─ T12: Collection Manager (depends: T11, T4)
├─ T13: Upsert Handler (depends: T11, T3)
└─ T14: Search Handler (depends: T11, T3, T5)

LAYER 5 (Query Translation)
└─ T15: Filter-to-Qdrant Translator (depends: T5, T11)

LAYER 6 (Applications)
├─ T16: Ingestion CLI (depends: T9, T10, T13)
└─ T17: Search Server (depends: T10, T14, T15)

LAYER 7 (Validation)
├─ T18: Test Fixtures
└─ T19: Integration Tests (depends: ALL)
```

---

## Task Specifications

### T1: Docker/Qdrant Setup
**Owner**: DevOps/Infrastructure
**Dependencies**: None
**Parallel**: Can run immediately

**Contract**:
- Input: None
- Output: Running Qdrant instance at `localhost:6333`

**Implementation**:
- `docker-compose.yml` with Qdrant service
- Volume persistence at `./qdrant_data`

**Acceptance Criteria**:
```bash
# MUST pass
curl -f http://localhost:6333/collections || exit 1
```

**Tests**:
- Health check returns 200
- Collections endpoint accessible

**Edge Cases (Fail-Fast)**:
- Port 6333 already in use → HALT with error
- Docker not running → HALT with error

---

### T2: Go Module Initialization
**Owner**: Any
**Dependencies**: None
**Parallel**: Can run immediately

**Acceptance Criteria** (IMMUTABLE):

Accepted when:
- go.mod exists and go mod tidy / go mod verify succeed.
- Only directories required by downstream tasks exist.
- No business logic, configuration, logging, flags, or placeholders exist.
- All directories are empty or contain package declarations only.
- No references to embeddings, storage, servers, MCP, or future tasks.

Failure = task rejected.

---

### T3: Canonical Document Model
**Owner**: Data Architect
**Dependencies**: T2
**Parallel**: No (blocking task for most others)

**Contract**:
- Input: None
- Output: `internal/model/document.go`

**Implementation**:
```go
// Strict schema - no optional fields except metadata
type Document struct {
    ID        string            `json:"id"`         // MUST be non-empty
    Source    string            `json:"source"`     // MUST be non-empty
    Type      string            `json:"type"`       // MUST be non-empty
    Timestamp int64             `json:"timestamp_unix_ms"` // MUST be > 0
    Text      string            `json:"text"`       // MUST be non-empty
    Metadata  map[string]any    `json:"metadata"`   // MAY be empty
    Keywords  []string          `json:"keywords,omitempty"`
}

// Validate MUST be called before any Document is used
func (d *Document) Validate() error {
    if d.ID == "" { return ErrMissingID }
    if d.Source == "" { return ErrMissingSource }
    if d.Type == "" { return ErrMissingType }
    if d.Timestamp <= 0 { return ErrInvalidTimestamp }
    if d.Text == "" { return ErrEmptyText }
    return nil
}
```

**Acceptance Criteria**:
- Document with empty ID → validation fails
- Document with zero timestamp → validation fails
- Valid document → validation passes
- JSON roundtrip preserves all fields

**Tests**:
```go
func TestDocumentValidation(t *testing.T) {
    // MUST panic or error on invalid
    invalid := Document{ID: ""}
    if invalid.Validate() == nil { t.Fatal("accepted invalid doc") }

    // MUST accept valid
    valid := Document{ID: "x", Source: "s", Type: "t", Timestamp: 1, Text: "txt"}
    if valid.Validate() != nil { t.Fatal("rejected valid doc") }
}
```

**Edge Cases (Fail-Fast)**:
- Missing required field → Error
- Negative timestamp → Error
- Whitespace-only text → Error

---

### T4: Embedder Interface
**Owner**: ML Engineer
**Dependencies**: T2
**Parallel**: Yes (with T3, T5)

**Contract**:
```go
// internal/embed/embedder.go
type Embedder interface {
    // Dim MUST return >0
    Dim() int

    // Embed MUST:
    // - Return len(vectors) == len(texts)
    // - Return len(vectors[i]) == Dim() for all i
    // - Error if any text is empty
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

**Acceptance Criteria**:
- Interface compiles
- Mock implementation passes contract tests

**Tests**:
```go
func TestEmbedderContract(t *testing.T, e Embedder) {
    // MUST error on empty input
    _, err := e.Embed(ctx, []string{""})
    if err == nil { t.Fatal("accepted empty text") }

    // MUST return correct dimensions
    vecs, _ := e.Embed(ctx, []string{"test"})
    if len(vecs[0]) != e.Dim() { t.Fatal("dimension mismatch") }
}
```

---

### T5: Filter Model
**Owner**: Backend Engineer
**Dependencies**: T2
**Parallel**: Yes (with T3, T4)

**Contract**:
```go
// internal/model/filters.go
type DateRange struct {
    From string `json:"from"` // MUST be YYYY-MM-DD or empty
    To   string `json:"to"`   // MUST be YYYY-MM-DD or empty
}

func (d *DateRange) Validate() error {
    if d.From != "" && !isValidDate(d.From) { return ErrInvalidDate }
    if d.To != "" && !isValidDate(d.To) { return ErrInvalidDate }
    return nil
}

type Filters struct {
    Source   []string   `json:"source,omitempty"`
    Date     *DateRange `json:"date,omitempty"`
    Keywords []string   `json:"keywords,omitempty"`
    Author   string     `json:"author,omitempty"`
    ThreadID string     `json:"thread_id,omitempty"`
}

func (f *Filters) Validate() error { /* date validation only */ }
```

**Acceptance Criteria**:
- Invalid date format → validation fails
- `2024-13-01` → validation fails
- `2024-12-31` → validation passes

**Tests**:
```go
func TestFilterValidation(t *testing.T) {
    // MUST reject bad dates
    f := Filters{Date: &DateRange{From: "invalid"}}
    if f.Validate() == nil { t.Fatal("accepted bad date") }
}
```

---

### T6: Document ID Generator
**Owner**: Backend Engineer
**Dependencies**: T2
**Parallel**: Yes (with T7, T8)

**Acceptance Criteria** (IMMUTABLE):

Accepted when:
- Exposes a single pure function: StableID(conversationID, messageID string) string
- Panics if any input string is empty.
- Deterministically returns the same ID for the same inputs across runs.
- Has no dependencies on Document, adapters, or external state.

Tests:
- Same inputs → same output.
- Empty input → panic.
- No error returns. No recovery logic.

---

### T7: Text Canonicalizer
**Owner**: NLP Engineer
**Dependencies**: T2
**Parallel**: Yes (with T6, T8)

**Acceptance Criteria** (IMMUTABLE):

Accepted when:
- Accepts raw message content and returns canonicalized text.
- Performs only normalization (whitespace, formatting).
- Returns empty string for whitespace-only input.
- Does not validate, skip, log, or error.
- Has no knowledge of Documents, validation rules, or ingestion policy.

Tests:
- Formatting normalization works.
- Whitespace-only input → empty string.
- No panic. No error.

---

### T8: ChatGPT Export Parser
**Owner**: Data Engineer
**Dependencies**: None
**Parallel**: Yes

**Contract**:
```go
// internal/chatgpt/export.go
type Export struct {
    Conversations []Conversation `json:"conversations"`
}

// LoadExport MUST:
// - Return error if file doesn't exist
// - Return error if JSON is malformed
// - Handle both {conversations:[]} and [] formats
func LoadExport(path string) (*Export, error)
```

**Acceptance Criteria**:
- Valid JSON → parses successfully
- Invalid JSON → returns error
- Missing file → returns error

**Tests**:
- Test with both JSON formats
- Test with malformed JSON
- Test with non-existent file

---

### T9: ChatGPT Adapter
**Owner**: Data Engineer
**Dependencies**: T3, T6, T7, T8
**Parallel**: No

**Acceptance Criteria** (IMMUTABLE):

Accepted when:
- Is the sole enforcement point for Document.Validate().
- Constructs Document only after validation passes.
- Treats empty Text as invalid.
- Returns an error on validation failure.
- Never constructs or emits invalid Documents.
- Downstream code may assume all Documents are valid forever.

Tests:
- Valid input → valid Document.
- Empty text → error (not panic).
- Invalid metadata → error.
- No duplicate validation elsewhere.

**System-Level Invariant** (NON-NEGOTIABLE):
- All Documents are born valid.
- Validation happens exactly once (T9).
- Any violation after T9 is a programmer bug.

---

### T10: OpenAI Embedder
**Owner**: ML Engineer
**Dependencies**: T4
**Parallel**: Yes (with T11)

**Contract**:
- Implements `Embedder` interface
- MUST error if API key missing
- MUST error if API returns non-200
- MUST return vectors with correct dimension

**Acceptance Criteria**:
- Missing API key → error on Embed()
- Valid input → correct dimension vectors
- API error → propagates error

**Tests**:
- Mock HTTP client tests
- Dimension validation
- Error handling

---

### T11: Qdrant REST Client
**Owner**: Backend Engineer
**Dependencies**: None
**Parallel**: Yes

**Contract**:
```go
// internal/qdrant/client.go
type Client struct {
    BaseURL string
    HTTP    *http.Client
}

// do MUST:
// - Return error on non-2xx status
// - Set Content-Type: application/json
func (c *Client) do(ctx context.Context, method, path string, payload, out any) error
```

**Acceptance Criteria**:
- 200 response → success
- 400 response → error
- Network timeout → error

**Tests** (requires T1 running):
```go
func TestQdrantClient(t *testing.T) {
    c := qdrant.New("http://localhost:6333")
    err := c.do(ctx, "GET", "/collections", nil, nil)
    if err != nil { t.Fatal(err) }
}
```

---

### T12: Collection Manager
**Owner**: Backend Engineer
**Dependencies**: T11, T4
**Parallel**: Yes (with T13, T14)

**Contract**:
```go
// EnsureCollection MUST:
// - Create collection if not exists
// - Validate dim > 0
// - Use Cosine distance
func (c *Client) EnsureCollection(ctx context.Context, name string, dim int) error {
    if dim <= 0 { return ErrInvalidDimension }
    // ...
}
```

**Acceptance Criteria**:
- Collection exists → no-op
- Collection missing → creates with correct config
- Invalid dim → error

---

### T13: Upsert Handler
**Owner**: Backend Engineer
**Dependencies**: T11, T3
**Parallel**: Yes (with T12, T14)

**Contract**:
```go
// Upsert MUST:
// - Validate all points before sending
// - Use wait=true for synchronous writes
// - Return error if any point invalid
func (c *Client) Upsert(ctx context.Context, collection string, points []Point) error
```

**Acceptance Criteria**:
- Valid points → successful write
- Invalid vector dimension → error
- Empty ID → error

---

### T14: Search Handler
**Owner**: Backend Engineer
**Dependencies**: T11, T3, T5
**Parallel**: Yes (with T12, T13)

**Contract**:
```go
// Search MUST:
// - Validate vector dimension matches collection
// - Return empty results if no matches
// - Results sorted by score descending
func (c *Client) Search(ctx context.Context, collection string, req SearchRequest) (*SearchResponse, error)
```

**Acceptance Criteria**:
- Valid search → sorted results
- No matches → empty array (not error)
- Wrong dimension → error

---

### T15: Filter-to-Qdrant Translator
**Owner**: Backend Engineer
**Dependencies**: T5, T11
**Parallel**: No

**Contract**:
```go
// BuildQdrantFilter MUST:
// - Convert date strings to unix ms
// - Handle nil filters → nil output
// - Validate all date strings before conversion
func BuildQdrantFilter(f *model.Filters) (*qdrant.Filter, error)
```

**Acceptance Criteria**:
- `nil` filter → `nil` output
- Valid date range → correct unix ms bounds
- Invalid date → error

**Tests**:
```go
func TestBuildFilter(t *testing.T) {
    f := &model.Filters{
        Date: &model.DateRange{From: "2024-01-01", To: "2024-12-31"},
    }
    qf, err := BuildQdrantFilter(f)
    if err != nil { t.Fatal(err) }
    // Validate unix ms conversion
}
```

---

### T16: Ingestion CLI
**Owner**: Application Developer
**Dependencies**: T9, T10, T13
**Parallel**: Yes (with T17)

**Contract**:
- MUST validate all flags before processing
- MUST call Document.Validate() before embedding
- MUST halt on first error (fail-fast)

**Acceptance Criteria**:
```bash
# MUST fail on missing export
go run ./cmd/ingest_chatgpt --export "" && exit 1

# MUST fail on missing API key
go run ./cmd/ingest_chatgpt --export test.json --openai_key "" && exit 1

# MUST succeed on valid input
go run ./cmd/ingest_chatgpt --export fixtures/test.json --openai_key "$KEY" || exit 1
```

---

### T17: Search Server
**Owner**: Application Developer
**Dependencies**: T10, T14, T15
**Parallel**: Yes (with T16)

**Contract**:
- MUST validate request before processing
- MUST return 400 on invalid filter dates
- MUST return 400 on empty query
- MUST return 200 with empty results if no matches

**Acceptance Criteria**:
```bash
# MUST reject empty query
curl -X POST localhost:8080/search -d '{"query":""}' | grep -q "400"

# MUST accept valid request
curl -X POST localhost:8080/search -d '{"query":"test","limit":10}' | jq -e '.results'
```

---

### T18: Test Fixtures
**Owner**: QA Engineer
**Dependencies**: None
**Parallel**: Yes

**Deliverables**:
- `fixtures/chatgpt_export_valid.json` - Valid ChatGPT export
- `fixtures/chatgpt_export_malformed.json` - Invalid JSON
- `fixtures/chatgpt_export_empty.json` - Empty conversations array

**Acceptance Criteria**:
- Valid fixture parses successfully
- Malformed fixture produces parse error
- Empty fixture produces zero documents

---

### T19: Integration Tests
**Owner**: QA Engineer
**Dependencies**: ALL
**Parallel**: No

**Test Flow**:
```bash
#!/bin/bash
set -e  # Fail-fast

# 1. Start infrastructure
docker compose up -d
curl -f http://localhost:6333/collections || exit 1

# 2. Ingest test fixture
go run ./cmd/ingest_chatgpt \
  --export fixtures/chatgpt_export_valid.json \
  --openai_key "$OPENAI_API_KEY" \
  --collection test_docs || exit 1

# 3. Start server
go run ./cmd/server --collection test_docs &
SERVER_PID=$!
sleep 2

# 4. Run search
RESULT=$(curl -s -X POST localhost:8080/search \
  -d '{"query":"test query","limit":5}')

# 5. Validate response
echo "$RESULT" | jq -e '.results | length > 0' || exit 1

# 6. Cleanup
kill $SERVER_PID
docker compose down
```

**Acceptance Criteria**:
- All steps pass without errors
- Search returns expected results
- Re-running ingestion is idempotent

---

## Parallelization Strategy

### Wave 1 (Infrastructure - 30 min)
- T1 (DevOps)
- T2 (Any Engineer)

### Wave 2 (Contracts - 2 hours)
- T3 (Data Architect) ← BLOCKING
- T4 (ML Engineer)
- T5 (Backend Engineer)

### Wave 3 (Adapters - 4 hours)
**After T3 completes:**
- T6 (Backend Engineer A)
- T7 (NLP Engineer)
- T8 (Data Engineer A)

### Wave 4 (Implementation - 6 hours)
**After Wave 3:**
- T9 (Data Engineer A) - Requires T6, T7, T8
- T10 (ML Engineer) - Requires T4
- T11 (Backend Engineer B)

### Wave 5 (Storage - 4 hours)
**After T11:**
- T12 (Backend Engineer B)
- T13 (Backend Engineer C)
- T14 (Backend Engineer D)

### Wave 6 (Query & Apps - 4 hours)
**After Wave 5:**
- T15 (Backend Engineer A) - Requires T5, T11
- T16 (App Developer A) - Requires T9, T10, T13
- T17 (App Developer B) - Requires T10, T14, T15

### Wave 7 (Testing - Ongoing)
- T18 (QA) - Can start anytime, ready for Wave 6
- T19 (QA) - After all complete

---

## Testing Philosophy

### Unit Tests (Per Component)
- **Input validation only**
- No mocking internal logic
- Edge cases cause panics/errors

Example:
```go
// GOOD: Boundary test
func TestDocumentRejectsEmpty(t *testing.T) {
    d := Document{ID: ""}
    if d.Validate() == nil { t.Fatal("accepted invalid") }
}

// BAD: Internal logic test
func TestDocumentHashingAlgorithm(t *testing.T) {
    // Don't test SHA256 works - trust stdlib
}
```

### Integration Tests (End-to-End)
- Pipeline validation: Export → Search results
- No intermediate state inspection
- Fail on first error

### Contract Tests (Interfaces)
- Verify interface implementations satisfy contracts
- Run against all implementations

---

## Fail-Fast Patterns

### Input Validation
```go
func ProcessDocument(d *Document) error {
    if err := d.Validate(); err != nil {
        panic(fmt.Sprintf("invalid document: %v", err))
    }
    // Process...
}
```

### API Boundaries
```go
func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
    var req SearchRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json", 400)
        return
    }
    if req.Query == "" {
        http.Error(w, "query required", 400)
        return
    }
    // Continue...
}
```

### External Dependencies
```go
func (c *Client) do(ctx, method, path string, payload, out any) error {
    resp, err := c.HTTP.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    if resp.StatusCode >= 300 {
        return fmt.Errorf("http %s", resp.Status)
    }
    // Continue...
}
```

---

## Component Contracts Summary

| Component | Input | Output | Validation |
|-----------|-------|--------|------------|
| ChatGPT Parser | JSON file path | `Export` struct | Exists, valid JSON |
| ChatGPT Adapter | `Export` | `[]Document` | Each doc validates |
| Text Canonicalizer | Raw text | Normalized text | No empty after normalization |
| ID Generator | source, conv, msg | SHA256 hex | No empty inputs |
| Embedder | `[]string` | `[][]float32` | Dim matches, no empty |
| Qdrant Client | HTTP req | Response | Status 2xx |
| Filter Translator | `Filters` | `qdrant.Filter` | Valid dates |
| Search API | `SearchRequest` | `SearchResponse` | Non-empty query |

---

## Success Metrics

- **Zero silent failures**: Every error logs and halts
- **Zero flaky tests**: All tests deterministic
- **Zero cross-component testing**: Each component tested in isolation
- **Sub-200ms queries**: Measured in integration tests
- **Idempotent ingestion**: Re-run produces identical results
