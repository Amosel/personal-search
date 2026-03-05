## 0) What you get (scope)

* ChatGPT export adapter: **export.json → Documents**
* Embedding generation: pluggable (OpenAI example + interface; swap to local later)
* Qdrant storage: vectors + payload filters
* Search API: `POST /search` (MCP-friendly contract)
* Deterministic IDs and re-ingestion safe upserts

### Current Repository Status (March 2026)

Implemented in repo:
- `cmd/ingest_chatgpt` end-to-end ingest (ChatGPT export -> embeddings -> Qdrant upsert)
- `cmd/server` with `/health` and `POST /search`
- Qdrant client/collection/upsert/search helpers in `internal/qdrant`
- Integration acceptance suite in `internal/integration`

Available embedder modes:
- `openai` for real semantic embeddings
- `fake` deterministic embedder for local integration tests and CI

### Source Boundary

ChatGPT export ingestion is split into two layers:

* **Source layer**: discovers and streams raw conversation data (`chatgpt_export_source.md`)
* **Adapter layer**: interprets streamed data into validated Documents

The source layer MUST NOT perform validation, filtering, or semantic decisions.

---

## 1) Run Qdrant locally (Docker)

```yaml
# docker-compose.yml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage
volumes:
  qdrant_data:
```

Start:

```bash
docker compose up -d
```

---

## 2) Go module layout

```
personal-search/
  go.mod
  docker-compose.yml
  cmd/
    server/
      main.go
    ingest_chatgpt/
      main.go
  internal/
    model/
      document.go
      filters.go
    chatgpt/
      export.go
      adapter.go
      canonicalize.go
    embed/
      embedder.go
      openai.go
    qdrant/
      client.go
      collection.go
      upsert.go
      search.go
```

---

## 3) go.mod

```go
module personal-search

go 1.22
```

---

## 4) Canonical model

```go
// internal/model/document.go
package model

type Document struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"` // "chatgpt"
	Type      string            `json:"type"`   // "message"
	Timestamp int64             `json:"timestamp_unix_ms"`
	Text      string            `json:"text"`
	Metadata  map[string]any    `json:"metadata"`
	Keywords  []string          `json:"keywords,omitempty"` // optional (exact/derived)
}
```

```go
// internal/model/filters.go
package model

type DateRange struct {
	From string `json:"from,omitempty"` // YYYY-MM-DD
	To   string `json:"to,omitempty"`   // YYYY-MM-DD
}

type Filters struct {
	Source   []string  `json:"source,omitempty"`
	Date     *DateRange `json:"date,omitempty"`
	Keywords []string  `json:"keywords,omitempty"`
	Author   string    `json:"author,omitempty"`
	ThreadID string    `json:"thread_id,omitempty"`
}

type SearchRequest struct {
	Query   string   `json:"query"`
	Filters *Filters `json:"filters,omitempty"`
	Limit   int      `json:"limit,omitempty"`
}

type SearchResult struct {
	ID        string         `json:"id"`
	Score     float64        `json:"score"`
	Text      string         `json:"text"`
	Timestamp string         `json:"timestamp"` // ISO string for convenience
	Source    string         `json:"source"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}
```

---

## 5) ChatGPT export adapter (robust-enough parsing)

ChatGPT export JSON structure varies; this adapter aims for the common “conversations/messages mapping” format and skips unknown shapes safely.

```go
// internal/chatgpt/export.go
package chatgpt

import (
	"encoding/json"
	"fmt"
	"os"
)

type Export struct {
	Conversations []Conversation `json:"conversations"`
}

type Conversation struct {
	ID       string             `json:"id"`
	Title    string             `json:"title,omitempty"`
	Mapping  map[string]Message `json:"mapping,omitempty"`
}

type Message struct {
	ID         string     `json:"id"`
	Message    *MsgBody   `json:"message,omitempty"`
	CreateTime float64    `json:"create_time,omitempty"` // seconds
	Parent     string     `json:"parent,omitempty"`
}

type MsgBody struct {
	ID      string    `json:"id,omitempty"`
	Author  Author    `json:"author"`
	Content Content   `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Author struct {
	Role string `json:"role"` // "user" | "assistant" | "system"
}

type Content struct {
	ContentType string        `json:"content_type"` // "text"
	Parts       []any         `json:"parts"`        // often []string but can vary
	Text        string        `json:"text,omitempty"`
}

func LoadExport(path string) (*Export, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Some exports are just an array of conversations.
	var convs []Conversation
	if err := json.Unmarshal(b, &convs); err == nil && len(convs) > 0 {
		return &Export{Conversations: convs}, nil
	}

	var e Export
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("unrecognized export format: %w", err)
	}
	return &e, nil
}
```

Canonicalization:

```go
// internal/chatgpt/canonicalize.go
package chatgpt

import (
	"fmt"
	"strings"
)

func CanonicalizeText(m *MsgBody) string {
	if m == nil {
		return ""
	}

	// Prefer content.parts if present
	if len(m.Content.Parts) > 0 {
		var sb strings.Builder
		for _, p := range m.Content.Parts {
			switch v := p.(type) {
			case string:
				sb.WriteString(v)
			default:
				// best-effort stringify
				sb.WriteString(fmt.Sprint(v))
			}
			sb.WriteString("\n")
		}
		return normalizeWhitespace(sb.String())
	}

	if m.Content.Text != "" {
		return normalizeWhitespace(m.Content.Text)
	}

	return ""
}

func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// trim excessive blank lines
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		if strings.TrimSpace(ln) == "" {
			blank++
			if blank <= 1 {
				out = append(out, "")
			}
			continue
		}
		blank = 0
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
```

Adapter:

```go
// internal/chatgpt/adapter.go
package chatgpt

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"personal-search/internal/model"
)

func StableID(conversationID, messageID string) string {
	h := sha256.Sum256([]byte("chatgpt|" + conversationID + "|" + messageID))
	return hex.EncodeToString(h[:])
}

func ToDocuments(e *Export) ([]model.Document, error) {
	var docs []model.Document

	for _, c := range e.Conversations {
		// Gather messages from mapping, sort by create_time for determinism
		var msgs []Message
		for _, m := range c.Mapping {
			if m.Message == nil || m.Message.Author.Role == "system" {
				continue
			}
			// Only keep messages with text
			txt := CanonicalizeText(m.Message)
			if txt == "" {
				continue
			}
			msgs = append(msgs, m)
		}
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].CreateTime < msgs[j].CreateTime })

		for _, m := range msgs {
			ts := int64(m.CreateTime * 1000)
			iso := time.UnixMilli(ts).UTC().Format(time.RFC3339)

			md := map[string]any{
				"author":    m.Message.Author.Role, // "user" | "assistant"
				"thread_id": c.ID,
				"title":     c.Title,
				"created":   iso,
			}

			docs = append(docs, model.Document{
				ID:        StableID(c.ID, m.ID),
				Source:    "chatgpt",
				Type:      "message",
				Timestamp: ts,
				Text:      CanonicalizeText(m.Message),
				Metadata:  md,
			})
		}
	}

	return docs, nil
}
```

---

## 6) Embedder interface + OpenAI example

If you prefer local embeddings later, implement the same interface.

```go
// internal/embed/embedder.go
package embed

import "context"

type Embedder interface {
	Dim() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

```go
// internal/embed/openai.go
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAIEmbedder struct {
	APIKey string
	Model  string
	DimVal int
	Client *http.Client
}

func (o *OpenAIEmbedder) Dim() int { return o.DimVal }

type oaiReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type oaiResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (o *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}

	body, _ := json.Marshal(oaiReq{Model: o.Model, Input: texts})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai embeddings status: %s", resp.Status)
	}

	var out oaiResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	embs := make([][]float32, len(texts))
	for _, d := range out.Data {
		embs[d.Index] = d.Embedding
	}
	return embs, nil
}
```

> Set `DimVal` to the expected embedding dimension for the model you choose.

---

## 7) Qdrant client (REST, minimal)

```go
// internal/qdrant/client.go
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, payload any, out any) error {
	var body *bytes.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant %s %s -> %s", method, path, resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
```

Create collection:

```go
// internal/qdrant/collection.go
package qdrant

import "context"

type CreateCollectionRequest struct {
	Vectors struct {
		Size     int    `json:"size"`
		Distance string `json:"distance"` // "Cosine"
	} `json:"vectors"`
}

func (c *Client) EnsureCollection(ctx context.Context, name string, dim int) error {
	// Check existence
	var exists any
	if err := c.do(ctx, "GET", "/collections/"+name, nil, &exists); err == nil {
		return nil
	}

	req := CreateCollectionRequest{}
	req.Vectors.Size = dim
	req.Vectors.Distance = "Cosine"
	return c.do(ctx, "PUT", "/collections/"+name, req, nil)
}
```

Upsert points:

```go
// internal/qdrant/upsert.go
package qdrant

import (
	"context"
)

type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type UpsertRequest struct {
	Points []Point `json:"points"`
}

func (c *Client) Upsert(ctx context.Context, collection string, points []Point) error {
	req := UpsertRequest{Points: points}
	return c.do(ctx, "PUT", "/collections/"+collection+"/points?wait=true", req, nil)
}
```

Search:

```go
// internal/qdrant/search.go
package qdrant

import "context"

type MatchValue struct {
	Value any `json:"value"`
}

type FieldCondition struct {
	Key   string      `json:"key"`
	Match *MatchValue `json:"match,omitempty"`
	Range *Range      `json:"range,omitempty"`
}

type Range struct {
	Gte *float64 `json:"gte,omitempty"`
	Lte *float64 `json:"lte,omitempty"`
}

type Filter struct {
	Must []any `json:"must,omitempty"`
}

type SearchRequest struct {
	Vector       []float32 `json:"vector"`
	Limit        int       `json:"limit"`
	WithPayload  bool      `json:"with_payload"`
	Filter       *Filter   `json:"filter,omitempty"`
}

type SearchResponse struct {
	Result []struct {
		ID     any            `json:"id"`
		Score  float64        `json:"score"`
		Payload map[string]any `json:"payload"`
	} `json:"result"`
}

func (c *Client) Search(ctx context.Context, collection string, req SearchRequest) (*SearchResponse, error) {
	var out SearchResponse
	if err := c.do(ctx, "POST", "/collections/"+collection+"/points/search", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

---

## 8) Ingest command (ChatGPT export → embeddings → Qdrant)

```go
// cmd/ingest_chatgpt/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
)

func main() {
	var (
		exportPath = flag.String("export", "", "Path to ChatGPT export JSON")
		qdrantURL  = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
		collection = flag.String("collection", "personal_docs", "Qdrant collection name")
		openaiKey  = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
		modelName  = flag.String("model", "text-embedding-3-large", "Embedding model")
		dim        = flag.Int("dim", 3072, "Embedding dimension for the model")
		batchSize  = flag.Int("batch", 64, "Embedding batch size")
	)
	flag.Parse()

	if *exportPath == "" {
		panic("missing --export")
	}
	if *openaiKey == "" {
		panic("missing OpenAI key (set OPENAI_API_KEY or --openai_key)")
	}

	ctx := context.Background()

	exp, err := chatgpt.LoadExport(*exportPath)
	if err != nil {
		panic(err)
	}
	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Parsed %d documents\n", len(docs))

	emb := &embed.OpenAIEmbedder{
		APIKey: *openaiKey,
		Model:  *modelName,
		DimVal: *dim,
	}

	qc := qdrant.New(*qdrantURL)
	if err := qc.EnsureCollection(ctx, *collection, emb.Dim()); err != nil {
		panic(err)
	}

	// Batch embed + upsert
	for i := 0; i < len(docs); i += *batchSize {
		j := i + *batchSize
		if j > len(docs) {
			j = len(docs)
		}
		batch := docs[i:j]

		texts := make([]string, len(batch))
		for k := range batch {
			texts[k] = batch[k].Text
		}

		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			panic(err)
		}

		points := make([]qdrant.Point, 0, len(batch))
		for k, d := range batch {
			payload := map[string]any{
				"source":            d.Source,
				"type":              d.Type,
				"timestamp_unix_ms": float64(d.Timestamp),
				"text":              d.Text,
			}
			// Flatten metadata into payload for filtering
			for mk, mv := range d.Metadata {
				payload[mk] = mv
			}
			points = append(points, qdrant.Point{
				ID:      d.ID,
				Vector:  vecs[k],
				Payload: payload,
			})
		}

		if err := qc.Upsert(ctx, *collection, points); err != nil {
			panic(err)
		}
		fmt.Printf("Upserted %d/%d\n", j, len(docs))
	}

	fmt.Println("Done.")
}
```

---

## 9) Search server (MCP tool surface)

This server exposes `POST /search` with your schema.

```go
// cmd/server/main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"time"

	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

func main() {
	var (
		addr       = flag.String("addr", ":8080", "listen address")
		qdrantURL  = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
		collection = flag.String("collection", "personal_docs", "Qdrant collection name")
		openaiKey  = flag.String("openai_key", "", "OpenAI API key (for query embeddings)")
		modelName  = flag.String("model", "text-embedding-3-large", "Embedding model")
		dim        = flag.Int("dim", 3072, "Embedding dimension")
	)
	flag.Parse()

	emb := &embed.OpenAIEmbedder{APIKey: *openaiKey, Model: *modelName, DimVal: *dim}
	qc := qdrant.New(*qdrantURL)

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req model.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Limit <= 0 {
			req.Limit = 20
		}
		if req.Query == "" {
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}
		if emb.APIKey == "" {
			http.Error(w, "server missing embedding key for query embedding", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		qvecs, err := emb.Embed(ctx, []string{req.Query})
		if err != nil {
			http.Error(w, "embedding error", http.StatusInternalServerError)
			return
		}

		filter := buildQdrantFilter(req.Filters)

		sreq := qdrant.SearchRequest{
			Vector:      qvecs[0],
			Limit:       req.Limit,
			WithPayload: true,
			Filter:      filter,
		}

		out, err := qc.Search(context.Background(), *collection, sreq)
		if err != nil {
			http.Error(w, "qdrant search error", http.StatusInternalServerError)
			return
		}

		resp := model.SearchResponse{Results: make([]model.SearchResult, 0, len(out.Result))}
		for _, hit := range out.Result {
			tsMs, _ := hit.Payload["timestamp_unix_ms"].(float64)
			tISO := time.UnixMilli(int64(tsMs)).UTC().Format(time.RFC3339)

			text, _ := hit.Payload["text"].(string)
			source, _ := hit.Payload["source"].(string)

			// Preserve payload as metadata, but you may want to redact "text" duplication later.
			md := hit.Payload

			resp.Results = append(resp.Results, model.SearchResult{
				ID:        toString(hit.ID),
				Score:     hit.Score,
				Text:      text,
				Timestamp: tISO,
				Source:    source,
				Metadata:  md,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	_ = qc.EnsureCollection(context.Background(), *collection, emb.Dim())
	http.ListenAndServe(*addr, nil)
}

func toString(id any) string {
	switch v := id.(type) {
	case string:
		return v
	default:
		// qdrant may return numeric ids; we use string ids, so this is defensive
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func buildQdrantFilter(f *model.Filters) *qdrant.Filter {
	if f == nil {
		return nil
	}

	var must []any

	// source in [...]
	if len(f.Source) > 0 {
		// Qdrant supports "should" for OR; simplest: choose one value or implement should.
		// For reference simplicity, if multiple sources provided, we skip (extend to should in next iteration).
		must = append(must, qdrant.FieldCondition{
			Key:   "source",
			Match: &qdrant.MatchValue{Value: f.Source[0]},
		})
	}

	// author exact
	if f.Author != "" {
		must = append(must, qdrant.FieldCondition{
			Key:   "author",
			Match: &qdrant.MatchValue{Value: f.Author},
		})
	}

	// thread id exact
	if f.ThreadID != "" {
		must = append(must, qdrant.FieldCondition{
			Key:   "thread_id",
			Match: &qdrant.MatchValue{Value: f.ThreadID},
		})
	}

	// date range: convert YYYY-MM-DD -> unix ms bounds
	if f.Date != nil && (f.Date.From != "" || f.Date.To != "") {
		var rng qdrant.Range
		if f.Date.From != "" {
			if t, err := time.Parse("2006-01-02", f.Date.From); err == nil {
				v := float64(t.UTC().UnixMilli())
				rng.Gte = &v
			}
		}
		if f.Date.To != "" {
			// inclusive end-of-day
			if t, err := time.Parse("2006-01-02", f.Date.To); err == nil {
				end := t.Add(24*time.Hour - time.Millisecond).UTC()
				v := float64(end.UnixMilli())
				rng.Lte = &v
			}
		}
		must = append(must, qdrant.FieldCondition{
			Key:   "timestamp_unix_ms",
			Range: &rng,
		})
	}

	// keywords: reference implementation expects payload["keywords"] as array; not set yet.
	// You can add keyword extraction later; for now treat as no-op or implement lexical layer.
	// (Kept here to preserve contract.)

	if len(must) == 0 {
		return nil
	}
	return &qdrant.Filter{Must: must}
}
```

---

## 10) MCP tool spec (what you register)

This is the tool surface your agent calls (your server implements it).

```json
{
  "name": "search_documents",
  "description": "Semantic search over personal indexed documents with optional deterministic filters (source/date/author/thread/keywords).",
  "inputSchema": {
    "type": "object",
    "properties": {
      "query": { "type": "string" },
      "filters": {
        "type": "object",
        "properties": {
          "source": { "type": "array", "items": { "type": "string" } },
          "date": {
            "type": "object",
            "properties": {
              "from": { "type": "string", "description": "YYYY-MM-DD" },
              "to":   { "type": "string", "description": "YYYY-MM-DD" }
            }
          },
          "keywords": { "type": "array", "items": { "type": "string" } },
          "author": { "type": "string" },
          "thread_id": { "type": "string" }
        }
      },
      "limit": { "type": "integer", "default": 20 }
    },
    "required": ["query"]
  }
}
```

---

## 11) How to run

1. Start Qdrant

```bash
docker compose up -d
```

If this repository does not yet include a committed `docker-compose.yml`, use:

```bash
docker run -d --name personal-search-qdrant -p 6333:6333 qdrant/qdrant:latest
```

2. Ingest ChatGPT export

```bash
go run ./cmd/ingest_chatgpt --export /path/to/chatgpt_export.json --openai_key "$OPENAI_API_KEY" --dim 3072
```

For deterministic local test mode (no OpenAI key):

```bash
go run ./cmd/ingest_chatgpt \
  --export internal/integration/testdata/chatgpt_export_valid.json \
  --embedder fake \
  --dim 16 \
  --batch 2
```

3. Run server

```bash
go run ./cmd/server --openai_key "$OPENAI_API_KEY" --dim 3072
```

Deterministic local test mode:

```bash
go run ./cmd/server --embedder fake --dim 16
```

4. Query

```bash
curl -s localhost:8080/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"parental alienation patterns","filters":{"source":["chatgpt"],"date":{"from":"2024-01-01","to":"2024-12-31"}},"limit":10}' | jq
```

---

## 11.1 Acceptance Test Gate

```bash
# No cached test results
go test ./internal/integration -run TestAcceptance_E2E -v -count=1
```

This gate validates:
- ingest -> search happy path
- request validation errors
- deterministic top result (fake embedder)
- idempotent re-ingest
- source/author/thread/date filters
- empty collection behavior

---

## 12) Known limitations (intentional, next increments)

* `filters.source` currently uses only the first value (extend to Qdrant `should` OR clause)
* `keywords` filter is stubbed (add keyword extraction + payload)
* No lexical BM25 fallback yet (add later for exact match anchoring)
* Query embedding uses OpenAI (swap to local embedder once desired)
