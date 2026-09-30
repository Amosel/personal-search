package qdrant

import (
	"context"
	"fmt"
	"hash/fnv"

	"personal-search/internal/model"
)

// DocumentStore maps canonical Documents and embeddings to Qdrant points.
type DocumentStore struct {
	Client *Client
}

func NewDocumentStore(client *Client) *DocumentStore {
	return &DocumentStore{Client: client}
}

func (s *DocumentStore) EnsureCollection(ctx context.Context, name string, dim int) error {
	if s == nil || s.Client == nil {
		return fmt.Errorf("qdrant client is required")
	}
	return s.Client.EnsureCollection(ctx, name, dim)
}

func (s *DocumentStore) Upsert(ctx context.Context, collection string, docs []model.Document, vectors [][]float32) error {
	if s == nil || s.Client == nil {
		return fmt.Errorf("qdrant client is required")
	}
	if len(docs) != len(vectors) {
		return fmt.Errorf("document/vector count mismatch: %d documents, %d vectors", len(docs), len(vectors))
	}
	points := make([]Point, 0, len(docs))
	for i, doc := range docs {
		payload := map[string]any{
			"doc_id": doc.ID, "source": doc.Source, "type": doc.Type,
			"timestamp_unix_ms": float64(doc.Timestamp), "text": doc.Text,
		}
		for key, value := range doc.Metadata {
			payload[key] = value
		}
		points = append(points, Point{ID: PointIDFromDocID(doc.ID), Vector: vectors[i], Payload: payload})
	}
	return s.Client.Upsert(ctx, collection, points)
}

func PointIDFromDocID(docID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	return h.Sum64()
}
