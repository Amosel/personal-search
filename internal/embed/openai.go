package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIEmbedder implements Embedder using OpenAI's API.
type OpenAIEmbedder struct {
	APIKey string
	Model  string
	DimVal int
	Client *http.Client
}

// Dim returns the embedding dimension.
func (o *OpenAIEmbedder) Dim() int {
	return o.DimVal
}

type openAIRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed generates embeddings for the provided texts.
// Returns error if API key is missing, any text is empty, or API call fails.
func (o *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if o.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	// Validate inputs
	for i, text := range texts {
		if text == "" {
			return nil, fmt.Errorf("text at index %d is empty", i)
		}
	}

	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}

	reqBody := openAIRequest{
		Model: o.Model,
		Input: texts,
	}
	if o.DimVal > 0 {
		reqBody.Dimensions = &o.DimVal
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI API returned status %s", resp.Status)
	}

	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Build result slice
	embeddings := make([][]float32, len(texts))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("invalid index %d in response", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}

	// Verify all embeddings were returned
	for i, emb := range embeddings {
		if emb == nil {
			return nil, fmt.Errorf("missing embedding for index %d", i)
		}
		if len(emb) != o.DimVal {
			return nil, fmt.Errorf("embedding at index %d has dimension %d, expected %d", i, len(emb), o.DimVal)
		}
	}

	return embeddings, nil
}
