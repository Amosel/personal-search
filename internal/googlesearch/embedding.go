package googlesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"personal-search/internal/embed"
	"strings"
	"time"
)

// PinnedEmbedder identifies model contents, not a mutable Ollama tag.
type PinnedEmbedder struct {
	*embed.OllamaEmbedder
	Identity string
}

func NewEmbedder(ctx context.Context, baseURL, model string) (*PinnedEmbedder, error) {
	digest, err := modelDigest(ctx, baseURL, model)
	if err != nil {
		return nil, err
	}
	dim, err := embed.DetectOllamaDimension(ctx, baseURL, model, nil)
	if err != nil {
		return nil, err
	}
	after, err := modelDigest(ctx, baseURL, model)
	if err != nil {
		return nil, err
	}
	if after != digest {
		return nil, fmt.Errorf("Ollama model changed during dimension probe")
	}
	return &PinnedEmbedder{OllamaEmbedder: &embed.OllamaEmbedder{BaseURL: baseURL, Model: model, DimVal: dim}, Identity: "ollama:" + digest + ":clip2000:v1"}, nil
}
func (e *PinnedEmbedder) EmbeddingIdentity() string { return e.Identity }
func (e *PinnedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	digest, err := modelDigest(ctx, e.BaseURL, e.Model)
	if err != nil {
		return nil, err
	}
	if "ollama:"+digest+":clip2000:v1" != e.Identity {
		return nil, fmt.Errorf("Ollama model changed during operation")
	}
	vectors, err := e.OllamaEmbedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	after, err := modelDigest(ctx, e.BaseURL, e.Model)
	if err != nil {
		return nil, err
	}
	if after != digest {
		return nil, fmt.Errorf("Ollama model changed during embedding")
	}
	return vectors, nil
}
func modelDigest(ctx context.Context, baseURL, model string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama tags: %s", resp.Status)
	}
	var out struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !strings.Contains(model, ":") {
		model += ":latest"
	}
	for _, m := range out.Models {
		if m.Name == model && m.Digest != "" {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("Ollama model %q unavailable; model installation requires approval", model)
}
