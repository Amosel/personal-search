package embed

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestOpenAIEmbedder_Dim(t *testing.T) {
	embedder := &OpenAIEmbedder{DimVal: 1536}
	if embedder.Dim() != 1536 {
		t.Errorf("expected Dim() = 1536, got %d", embedder.Dim())
	}
}

func TestOpenAIEmbedder_MissingAPIKey(t *testing.T) {
	embedder := &OpenAIEmbedder{
		APIKey: "",
		Model:  "text-embedding-3-small",
		DimVal: 1536,
	}

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestOpenAIEmbedder_EmptyText(t *testing.T) {
	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "text-embedding-3-small",
		DimVal: 1536,
	}

	_, err := embedder.Embed(context.Background(), []string{""})
	if err == nil {
		t.Fatal("expected error for empty text")
	}

	_, err = embedder.Embed(context.Background(), []string{"valid", ""})
	if err == nil {
		t.Fatal("expected error for empty text in batch")
	}
}

func TestOpenAIEmbedder_RequestIncludesDimensions(t *testing.T) {
	rt := &recordingRoundTripper{
		statusCode: http.StatusOK,
		body:       `{"data":[{"embedding":[0.1,0.2,0.3],"index":0},{"embedding":[0.4,0.5,0.6],"index":1}]}`,
	}

	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "text-embedding-3-small",
		DimVal: 3,
		Client: &http.Client{Transport: rt},
	}

	embeddings, err := embedder.Embed(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if rt.authorization != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %s", rt.authorization)
	}
	if !bytes.Contains(rt.requestBody, []byte(`"dimensions":3`)) {
		t.Fatalf("expected dimensions in request body, got %s", string(rt.requestBody))
	}
}

func TestOpenAIEmbedder_MockAPI_NonOKStatus(t *testing.T) {
	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "text-embedding-3-small",
		DimVal: 3,
		Client: &http.Client{Transport: &recordingRoundTripper{
			statusCode: http.StatusBadRequest,
			body:       `{"error":"bad request"}`,
		}},
	}

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestOpenAIEmbedder_WrongDimension(t *testing.T) {
	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "test-model",
		DimVal: 3,
		Client: &http.Client{Transport: &recordingRoundTripper{
			statusCode: http.StatusOK,
			body:       `{"data":[{"embedding":[0.1,0.2],"index":0}]}`,
		}},
	}

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
}

type recordingRoundTripper struct {
	statusCode    int
	body          string
	requestBody   []byte
	authorization string
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var err error
	r.requestBody, err = io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	r.authorization = req.Header.Get("Authorization")

	return &http.Response{
		StatusCode: r.statusCode,
		Status:     http.StatusText(r.statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(r.body)),
		Request:    req,
	}, nil
}
