package embed

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestOpenAIEmbedder_MockAPI_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		response := `{
			"data": [
				{"embedding": [0.1, 0.2, 0.3], "index": 0},
				{"embedding": [0.4, 0.5, 0.6], "index": 1}
			]
		}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer server.Close()

	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "text-embedding-3-small",
		DimVal: 3,
		Client: &http.Client{},
	}

	// Override URL for testing
	embedder.Client.Transport = &mockTransport{server: server}

	embeddings, err := embedder.Embed(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}

	if len(embeddings[0]) != 3 {
		t.Errorf("expected dimension 3, got %d", len(embeddings[0]))
	}

	if embeddings[0][0] != 0.1 || embeddings[0][1] != 0.2 || embeddings[0][2] != 0.3 {
		t.Errorf("unexpected embedding values: %v", embeddings[0])
	}
}

func TestOpenAIEmbedder_MockAPI_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "text-embedding-3-small",
		DimVal: 3,
		Client: &http.Client{},
	}

	embedder.Client.Transport = &mockTransport{server: server}

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestOpenAIEmbedder_WrongDimension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"data": [
				{"embedding": [0.1, 0.2], "index": 0}
			]
		}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer server.Close()

	embedder := &OpenAIEmbedder{
		APIKey: "test-key",
		Model:  "test-model",
		DimVal: 3, // Expect 3 but API returns 2
		Client: &http.Client{},
	}

	embedder.Client.Transport = &mockTransport{server: server}

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
}

// mockTransport redirects all requests to the test server
type mockTransport struct {
	server *httptest.Server
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = m.server.URL[7:] // Remove "http://"
	return http.DefaultTransport.RoundTrip(req)
}
