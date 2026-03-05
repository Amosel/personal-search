package embed

import (
	"context"
	"fmt"
	"hash/fnv"
)

// FakeEmbedder is a deterministic, local embedder for tests and offline runs.
type FakeEmbedder struct {
	DimVal int
}

func (f *FakeEmbedder) Dim() int {
	return f.DimVal
}

func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.DimVal <= 0 {
		return nil, fmt.Errorf("fake embedder dimension must be > 0")
	}

	out := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("text at index %d is empty", i)
		}
		vec := make([]float32, f.DimVal)
		for _, tok := range splitWords(text) {
			h := fnv.New64a()
			_, _ = h.Write([]byte(tok))
			idx := int(h.Sum64() % uint64(f.DimVal))
			vec[idx]++
		}
		out[i] = vec
	}
	return out, nil
}

func splitWords(s string) []string {
	parts := make([]string, 0, len(s)/4)
	start := -1
	for i, r := range s {
		isWord := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if isWord {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			parts = append(parts, s[start:i])
			start = -1
		}
	}
	if start != -1 {
		parts = append(parts, s[start:])
	}
	return parts
}
