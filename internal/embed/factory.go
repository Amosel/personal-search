package embed

import (
	"context"
	"fmt"
)

const (
	DefaultOpenAIModel = "text-embedding-3-large"
	DefaultOllamaURL   = "http://localhost:11434"
	DefaultOllamaModel = "nomic-embed-text:latest"
)

// New returns a configured embedder for the requested provider.
// For Ollama, dim=0 means infer the model dimension from the local server.
func New(kind, openAIKey, openAIModel, ollamaURL, ollamaModel string, dim int) (Embedder, error) {
	switch kind {
	case "openai":
		if openAIKey == "" {
			return nil, fmt.Errorf("missing OpenAI key (set OPENAI_API_KEY or --openai_key)")
		}
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &OpenAIEmbedder{
			APIKey: openAIKey,
			Model:  openAIModel,
			DimVal: dim,
		}, nil
	case "fake":
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &FakeEmbedder{DimVal: dim}, nil
	case "ollama":
		if dim == 0 {
			var err error
			dim, err = DetectOllamaDimension(context.Background(), ollamaURL, ollamaModel, nil)
			if err != nil {
				return nil, fmt.Errorf("infer Ollama dimension: %w", err)
			}
		}
		if dim < 0 {
			return nil, fmt.Errorf("--dim must be >= 0")
		}
		return &OllamaEmbedder{
			BaseURL: ollamaURL,
			Model:   ollamaModel,
			DimVal:  dim,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported --embedder value: %s", kind)
	}
}
