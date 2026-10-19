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

	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
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

	resp, err := client.Do(req)
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

	if len(apiResp.Data) != len(texts) {
		return nil, &VectorError{Reason: "response count mismatch"}
	}
	seen := make(map[int]bool, len(texts))
	// Build result slice
	embeddings := make([][]float32, len(texts))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, &VectorError{Reason: fmt.Sprintf("invalid index %d in response", item.Index)}
		}
		if seen[item.Index] {
			return nil, &VectorError{Reason: "duplicate response index"}
		}
		seen[item.Index] = true
		embeddings[item.Index] = item.Embedding
	}

	if err := ValidateVectors(embeddings, len(texts), o.DimVal); err != nil {
		return nil, err
	}
	return embeddings, nil
}

func (o *OpenAIEmbedder) Profile() Profile {
	return Profile{Version: 1, Provider: "openai", ModelReference: o.Model, ModelRevision: o.Model, RevisionKind: "provider_reference", Dimensions: o.DimVal, DocumentTransform: "identity-v1", QueryTransform: "identity-v1"}
}
