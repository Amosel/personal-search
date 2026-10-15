package googlesearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

var errUncertainUpsert = errors.New("synthetic lost completion")

type boundaryStore struct {
	stored      map[any]qdrant.Point
	calls       int
	ensureCalls int
	failAt      int
	cancel      context.CancelFunc
}

func (s *boundaryStore) EnsureCollection(context.Context, string, int) error {
	s.ensureCalls++
	return nil
}

func (*boundaryStore) ValidateEmbedding(context.Context, string, string, int) error { return nil }

func (s *boundaryStore) Upsert(_ context.Context, _ string, points []qdrant.Point) error {
	s.calls++
	if s.stored == nil {
		s.stored = make(map[any]qdrant.Point)
	}
	for _, p := range points {
		s.stored[p.ID] = p
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.calls == s.failAt {
		return errUncertainUpsert
	}
	return nil
}

type boundaryEmbedder struct {
	testEmbedder
	calls  int
	fault  string
	cancel context.CancelFunc
}

func (e *boundaryEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	vectors, err := e.FakeEmbedder.Embed(ctx, texts)
	if e.calls != 2 {
		return vectors, err
	}
	switch e.fault {
	case "embed_error":
		return nil, context.DeadlineExceeded
	case "too_few":
		vectors = vectors[:1]
	case "too_many":
		vectors = append(vectors, make([]float32, 8))
	case "wrong_dimension":
		vectors[1] = make([]float32, 7)
	}
	if e.cancel != nil {
		e.cancel()
	}
	return vectors, nil
}

func syntheticDocuments(n int) []model.Document {
	docs := make([]model.Document, n)
	for i := range docs {
		d := doc()
		d.Text = fmt.Sprintf("synthetic query %d", i)
		d.ID = documentID("personal", d.Timestamp, d.Text)
		docs[i] = d
	}
	return docs
}

func TestRunEmbeddingFaultsPreserveAcknowledgedCount(t *testing.T) {
	for _, fault := range []string{"embed_error", "too_few", "too_many", "wrong_dimension"} {
		t.Run(fault, func(t *testing.T) {
			emb := &boundaryEmbedder{testEmbedder: testEmbedder{embed.FakeEmbedder{DimVal: 8}}, fault: fault}
			store := &boundaryStore{}
			n, err := Run(context.Background(), syntheticDocuments(5), "synthetic", 2, emb, store)
			if err == nil || n != 2 || store.calls != 1 || emb.calls != 2 || !strings.Contains(err.Error(), "embed batch [2:4]") {
				t.Fatalf("n=%d err=%v store_calls=%d embed_calls=%d", n, err, store.calls, emb.calls)
			}
			if fault == "embed_error" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("embedding error lost: %v", err)
			}
		})
	}
}

func TestRunUncertainWriteAndDeterministicRetry(t *testing.T) {
	store := &boundaryStore{failAt: 2}
	docs := syntheticDocuments(5)
	emb := &testEmbedder{embed.FakeEmbedder{DimVal: 8}}
	n, err := Run(context.Background(), docs, "synthetic", 2, emb, store)
	if n != 2 || !errors.Is(err, errUncertainUpsert) || !strings.Contains(err.Error(), "upsert batch [2:4]: completion unknown") || store.calls != 2 || len(store.stored) != 4 {
		t.Fatalf("n=%d err=%v store=%+v", n, err, store)
	}
	priorIDs := make(map[any]bool)
	for id := range store.stored {
		priorIDs[id] = true
	}
	store.failAt = 0
	n, err = Run(context.Background(), docs, "synthetic", 2, emb, store)
	if n != 5 || err != nil || len(store.stored) != 5 {
		t.Fatalf("retry n=%d err=%v store=%+v", n, err, store)
	}
	for id := range priorIDs {
		if _, ok := store.stored[id]; !ok {
			t.Fatalf("retry changed ID %v", id)
		}
	}
}

func TestRunCancellationPreservesAcknowledgements(t *testing.T) {
	for _, boundary := range []string{"before_store", "before_next_embed", "before_upsert", "after_final_acknowledgement"} {
		t.Run(boundary, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &boundaryStore{}
			emb := &boundaryEmbedder{testEmbedder: testEmbedder{embed.FakeEmbedder{DimVal: 8}}}
			count, want, wantCalls := 5, 0, 0
			switch boundary {
			case "before_store":
				cancel()
			case "before_next_embed":
				store.cancel = cancel
				want, wantCalls = 2, 1
			case "before_upsert":
				emb.cancel = cancel
				want, wantCalls = 2, 1
			case "after_final_acknowledgement":
				count, want, wantCalls = 2, 2, 1
				store.cancel = cancel
			}
			n, err := Run(ctx, syntheticDocuments(count), "synthetic", 2, emb, store)
			if !errors.Is(err, context.Canceled) || n != want || store.calls != wantCalls {
				t.Fatalf("n=%d want=%d err=%v store=%+v", n, want, err, store)
			}
			if boundary == "before_store" && (store.ensureCalls != 0 || emb.calls != 0) {
				t.Fatalf("service called before cancellation check: store=%+v embed_calls=%d", store, emb.calls)
			}
			if boundary == "before_next_embed" && emb.calls != 1 {
				t.Fatalf("next embed called after cancellation: calls=%d", emb.calls)
			}
		})
	}
}
