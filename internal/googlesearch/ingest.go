package googlesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strconv"
	"strings"
)

type Archive struct {
	SchemaVersion int              `json:"schema_version"`
	Source        string           `json:"source"`
	Documents     []model.Document `json:"documents"`
}

func Load(path string) ([]model.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var a Archive
	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := dec.Decode(&a); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data in archive")
	}
	if a.SchemaVersion != 1 || a.Source != "google_search" {
		return nil, fmt.Errorf("expected google_search archive schema 1")
	}
	if len(a.Documents) == 0 {
		return nil, fmt.Errorf("no search documents")
	}
	if err := ValidateDocuments(a.Documents); err != nil {
		return nil, err
	}

	return a.Documents, nil
}

// PointID uses a deterministic 128-bit UUID derived from the canonical event ID.
func PointID(id string) string {
	h := sha256.Sum256([]byte("google_search:" + id))
	v := hex.EncodeToString(h[:16])
	return v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
}

func ValidateDocuments(docs []model.Document) error {
	if len(docs) == 0 {
		return fmt.Errorf("no search documents")
	}
	seen := map[string]bool{}
	for i, d := range docs {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("document %d: %w", i, err)
		}
		if d.Source != "google_search" || d.Type != "search_query" || seen[d.ID] {
			return fmt.Errorf("invalid source/type or duplicate ID at document %d", i)
		}
		seen[d.ID] = true
		account, ok := d.Metadata["account"].(string)
		if !ok || strings.TrimSpace(account) == "" {
			return fmt.Errorf("document %d: account namespace required", i)
		}
		identity := "[\"google_search\"," + identityString(account) + "," + strconv.FormatInt(d.Timestamp, 10) + "," + identityString(d.Text) + "]"
		hash := sha256.Sum256([]byte(identity))
		if d.ID != hex.EncodeToString(hash[:]) {
			return fmt.Errorf("document %d: ID does not match account/timestamp/query identity", i)
		}
		if _, ok := d.Metadata["url"].(string); !ok {
			return fmt.Errorf("document %d: original URL field required", i)
		}
		if raw, ok := d.Metadata["raw"].(map[string]any); !ok || raw == nil {
			return fmt.Errorf("document %d: raw record required", i)
		}
		refs, ok := d.Metadata["provenance"].([]any)
		if !ok || len(refs) == 0 {
			return fmt.Errorf("document %d: provenance required", i)
		}
		for _, ref := range refs {
			r, ok := ref.(map[string]any)
			if !ok {
				return fmt.Errorf("document %d: invalid provenance", i)
			}
			locator, _ := r["locator"].(string)
			digest, _ := r["sha256"].(string)
			index, ok := recordIndex(r["record_index"])
			digestBytes, err := hex.DecodeString(digest)
			if locator == "" || err != nil || len(digestBytes) != 32 || !ok || index < 0 || index != float64(int64(index)) {
				return fmt.Errorf("document %d: invalid provenance locator/hash/index", i)
			}
		}
		for _, key := range []string{"doc_id", "source", "type", "timestamp_unix_ms", "text", "embedding_identity"} {
			if _, exists := d.Metadata[key]; exists {
				return fmt.Errorf("reserved metadata field: %s", key)
			}
		}
	}
	return nil
}

type Store interface {
	EnsureCollection(context.Context, string, int) error
	ValidateEmbedding(context.Context, string, string, int) error
	Upsert(context.Context, string, []qdrant.Point) error
}
type IdentifiedEmbedder interface {
	embed.Embedder
	EmbeddingIdentity() string
}

// Run returns only the count acknowledged by Qdrant, including on partial failure.
func Run(ctx context.Context, docs []model.Document, collection string, batch int, emb IdentifiedEmbedder, store Store) (int, error) {
	if batch <= 0 || collection == "" || emb == nil || store == nil {
		return 0, fmt.Errorf("collection, positive batch, embedder and store required")
	}
	if err := ValidateDocuments(docs); err != nil {
		return 0, err
	}
	if emb.EmbeddingIdentity() == "" {
		return 0, fmt.Errorf("embedding identity required")
	}
	if err := store.EnsureCollection(ctx, collection, emb.Dim()); err != nil {
		return 0, err
	}
	if err := store.ValidateEmbedding(ctx, collection, emb.EmbeddingIdentity(), emb.Dim()); err != nil {
		return 0, err
	}
	completed := 0
	for i := 0; i < len(docs); i += batch {
		end := i + batch
		if end > len(docs) {
			end = len(docs)
		}
		texts := make([]string, end-i)
		for j := i; j < end; j++ {
			texts[j-i] = docs[j].Text
		}
		vectors, err := emb.Embed(ctx, texts)
		if err != nil {
			return completed, fmt.Errorf("batch [%d:%d]: %w", i, end, err)
		}
		if len(vectors) != len(texts) {
			return completed, fmt.Errorf("embedding count mismatch")
		}
		points := make([]qdrant.Point, len(texts))
		for j := i; j < end; j++ {
			d := docs[j]
			vector := vectors[j-i]
			if len(vector) != emb.Dim() {
				return completed, fmt.Errorf("embedding dimension mismatch")
			}
			payload := map[string]any{}
			for k, v := range d.Metadata {
				payload[k] = v
			}
			payload["embedding_identity"] = emb.EmbeddingIdentity()
			payload["doc_id"] = d.ID
			payload["source"] = d.Source
			payload["type"] = d.Type
			payload["text"] = d.Text
			payload["timestamp_unix_ms"] = d.Timestamp
			points[j-i] = qdrant.Point{ID: PointID(d.ID), Vector: vector, Payload: payload}
		}
		if err := store.Upsert(ctx, collection, points); err != nil {
			return completed, fmt.Errorf("upsert batch [%d:%d]: completion unknown; may have been applied: %w", i, end, err)
		}
		completed += len(points)
	}
	return completed, nil
}

// Match the crawler's Python ensure_ascii=False canonical JSON, including U+2028.
func identityString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func recordIndex(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return float64(i), err == nil
	default:
		return 0, false
	}
}
