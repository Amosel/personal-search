package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PinnedOllama checks the content digest before and after probe/batch requests.
// This detects observed tag changes; it does not transactionally pin the server.
type PinnedOllama struct {
	backend OllamaEmbedder
	profile Profile
}

func NewPinnedOllama(ctx context.Context, baseURL, model string, dim int, client *http.Client) (*PinnedOllama, error) {
	if dim < 0 || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("valid URL, model and nonnegative dimension required")
	}
	if !strings.Contains(model, ":") {
		model += ":latest"
	}
	digest, err := modelDigest(ctx, baseURL, model, client)
	if err != nil {
		return nil, err
	}
	actual, err := DetectOllamaDimension(ctx, baseURL, model, client)
	if err != nil {
		return nil, err
	}
	if dim > 0 && dim != actual {
		return nil, fmt.Errorf("configured dimension %d differs from provider %d", dim, actual)
	}
	after, err := modelDigest(ctx, baseURL, model, client)
	if err != nil {
		return nil, err
	}
	if after != digest {
		return nil, fmt.Errorf("Ollama model changed during dimension probe")
	}
	p := Profile{Version: 1, Provider: "ollama", ModelReference: model, ModelRevision: digest, RevisionKind: "content_digest", Dimensions: actual, DocumentTransform: "headtail2000-v1", QueryTransform: "headtail2000-v1"}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &PinnedOllama{backend: OllamaEmbedder{BaseURL: baseURL, Model: model, DimVal: actual, Client: client}, profile: p}, nil
}
func (e *PinnedOllama) Dim() int         { return e.profile.Dimensions }
func (e *PinnedOllama) Profile() Profile { return e.profile }

// EmbeddingIdentity is retained only for legacy Google collection compatibility.
func (e *PinnedOllama) EmbeddingIdentity() string {
	return "ollama:" + e.profile.ModelRevision + ":clip2000:v1"
}
func (e *PinnedOllama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	digest, err := modelDigest(ctx, e.backend.BaseURL, e.backend.Model, e.backend.Client)
	if err != nil {
		return nil, err
	}
	if digest != e.profile.ModelRevision {
		return nil, fmt.Errorf("Ollama model changed during operation")
	}
	vectors, err := e.backend.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	after, err := modelDigest(ctx, e.backend.BaseURL, e.backend.Model, e.backend.Client)
	if err != nil {
		return nil, err
	}
	if after != digest {
		return nil, fmt.Errorf("Ollama model changed during embedding")
	}
	return vectors, nil
}
func modelDigest(ctx context.Context, baseURL, model string, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return "", err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
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
	for _, m := range out.Models {
		if m.Name == model && strings.TrimSpace(m.Digest) != "" {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("Ollama model %q unavailable locally", model)
}
