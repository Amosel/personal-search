package embed

import "context"

// Embedder generates vector embeddings for text.
type Embedder interface {
	// Dim returns the dimensionality of embeddings produced by this embedder.
	// MUST return a value > 0.
	Dim() int

	// Embed generates embeddings for the provided texts.
	// MUST return len(vectors) == len(texts).
	// MUST return len(vectors[i]) == Dim() for all i.
	// MUST return error if any text is empty.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
