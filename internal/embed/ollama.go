package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Keep Ollama inputs well below small embedding-model context limits.
// This preserves a representative head/tail slice while avoiding hard failures
// on very long assistant outputs in real ChatGPT exports.
const defaultOllamaMaxRunes = 2000

// OllamaEmbedder implements Embedder using a local Ollama server.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
	DimVal  int
	Client  *http.Client
}

func (o *OllamaEmbedder) Dim() int {
	return o.DimVal
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

func DetectOllamaDimension(ctx context.Context, baseURL, model string, client *http.Client) (int, error) {
	resp, err := doOllamaEmbed(ctx, baseURL, ollamaEmbedRequest{
		Model: model,
		Input: []string{"dimension probe"},
	}, client)
	if err != nil {
		return 0, err
	}
	if len(resp.Embeddings) != 1 || len(resp.Embeddings[0]) == 0 {
		return 0, fmt.Errorf("unable to infer Ollama embedding dimension")
	}
	return len(resp.Embeddings[0]), nil
}

func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(o.BaseURL) == "" {
		return nil, fmt.Errorf("Ollama base URL is required")
	}
	if strings.TrimSpace(o.Model) == "" {
		return nil, fmt.Errorf("Ollama model is required")
	}
	if o.DimVal <= 0 {
		return nil, fmt.Errorf("Ollama embedding dimension must be > 0")
	}
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("text at index %d is empty", i)
		}
	}
	inputs := make([]string, len(texts))
	for i, text := range texts {
		inputs[i] = clipForOllamaEmbedding(text, defaultOllamaMaxRunes)
	}

	out, err := doOllamaEmbed(ctx, o.BaseURL, ollamaEmbedRequest{
		Model: o.Model,
		Input: inputs,
	}, o.Client)
	if err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch got=%d want=%d", len(out.Embeddings), len(texts))
	}
	for i, emb := range out.Embeddings {
		if len(emb) != o.DimVal {
			return nil, fmt.Errorf("embedding at index %d has dimension %d, expected %d", i, len(emb), o.DimVal)
		}
	}
	return out.Embeddings, nil
}

func doOllamaEmbed(ctx context.Context, baseURL string, payload ollamaEmbedRequest, client *http.Client) (ollamaEmbedResponse, error) {
	var out ollamaEmbedResponse

	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return out, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("Ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return out, fmt.Errorf("Ollama returned status %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

func clipForOllamaEmbedding(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	head := int(float64(maxRunes) * 0.75)
	tail := maxRunes - head
	return string(runes[:head]) + "\n...\n" + string(runes[len(runes)-tail:])
}
