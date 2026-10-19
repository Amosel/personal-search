// Package applearchive adapts read-only Apple exports to the search index.
package applearchive

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"personal-search/internal/embed"
	"personal-search/internal/googlesearch"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

//go:embed export.py
var exporter string

type Archive struct {
	Documents []model.Document `json:"documents"`
	Report    map[string]any   `json:"report"`
}

func Load(ctx context.Context, source, path string) (Archive, error) {
	var archive Archive
	if (source != "apple_notes" && source != "voicememo") || path == "" {
		return archive, fmt.Errorf("source and input required")
	}
	cmd := exec.CommandContext(ctx, "python3", "-c", exporter, source, path)
	data, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return archive, fmt.Errorf("export: %s", strings.TrimSpace(string(e.Stderr)))
		}
		return archive, err
	}
	if err := json.Unmarshal(data, &archive); err != nil {
		return archive, err
	}
	return archive, Validate(archive.Documents, source)
}

func Validate(docs []model.Document, source string) error {
	if source != "apple_notes" && source != "voicememo" {
		return fmt.Errorf("unsupported source")
	}
	seen := map[string]bool{}
	for _, d := range docs {
		if err := d.Validate(); err != nil {
			return err
		}
		if d.Source != source || seen[d.ID] || strings.TrimSpace(d.Text) == "" {
			return fmt.Errorf("invalid source, duplicate ID or empty text")
		}
		if (source == "apple_notes" && d.Type != "note") || (source == "voicememo" && d.Type != "transcript") {
			return fmt.Errorf("invalid type")
		}
		seen[d.ID] = true
	}
	return nil
}

func pointID(id string) string {
	hash := sha256.Sum256([]byte(id))
	s := hex.EncodeToString(hash[:16])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func Run(ctx context.Context, docs []model.Document, source, collection string, batch int, emb googlesearch.IdentifiedEmbedder, store *qdrant.Client) (int, error) {
	if collection == "" || batch <= 0 || emb == nil || store == nil {
		return 0, fmt.Errorf("collection, positive batch, embedder and store required")
	}
	if err := Validate(docs, source); err != nil {
		return 0, err
	}
	if emb.EmbeddingIdentity() == "" {
		return 0, fmt.Errorf("embedding identity required")
	}
	if len(docs) == 0 {
		return 0, nil
	}
	if err := store.EnsureCollection(ctx, collection, emb.Dim()); err != nil {
		return 0, err
	}
	if err := store.ValidateSourceEmbedding(ctx, collection, emb.EmbeddingIdentity(), emb.Dim(), source); err != nil {
		return 0, err
	}
	completed := 0
	for start := 0; start < len(docs); start += batch {
		end := min(start+batch, len(docs))
		texts := make([]string, end-start)
		for i, d := range docs[start:end] {
			texts[i] = d.Text
		}
		vectors, err := emb.Embed(ctx, texts)
		if err != nil {
			return completed, err
		}
		if err := embed.ValidateVectors(vectors, len(texts), emb.Dim()); err != nil {
			return completed, err
		}
		points := make([]qdrant.Point, len(texts))
		for i, d := range docs[start:end] {
			payload := map[string]any{}
			for k, v := range d.Metadata {
				payload[k] = v
			}
			payload["doc_id"] = d.ID
			payload["source"] = source
			payload["type"] = d.Type
			payload["text"] = d.Text
			payload["timestamp_unix_ms"] = d.Timestamp
			payload["embedding_identity"] = emb.EmbeddingIdentity()
			id, err := qdrant.UUIDID(pointID(d.ID))
			if err != nil {
				return completed, err
			}
			points[i] = qdrant.Point{ID: id, Vector: vectors[i], Payload: payload}
		}
		if err := store.Upsert(ctx, collection, points); err != nil {
			return completed, fmt.Errorf("upsert completion unknown; may have been applied: %w", err)
		}
		completed += len(points)
	}
	return completed, nil
}
