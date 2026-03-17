package embed

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestOllamaEmbedder_ValidatesConfig(t *testing.T) {
	emb := &OllamaEmbedder{BaseURL: "", Model: "nomic-embed-text:latest", DimVal: 768}
	if _, err := emb.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("expected missing base URL error")
	}

	emb = &OllamaEmbedder{BaseURL: "http://localhost:11434", Model: "", DimVal: 768}
	if _, err := emb.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("expected missing model error")
	}

	emb = &OllamaEmbedder{BaseURL: "http://localhost:11434", Model: "nomic-embed-text:latest", DimVal: 0}
	if _, err := emb.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("expected missing dim error")
	}
}

func TestOllamaEmbedder_RequestAndResponse(t *testing.T) {
	rt := &ollamaRecordingRoundTripper{
		statusCode: http.StatusOK,
		body:       `{"model":"nomic-embed-text:latest","embeddings":[[0.1,0.2],[0.3,0.4]]}`,
	}
	emb := &OllamaEmbedder{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text:latest",
		DimVal:  2,
		Client:  &http.Client{Transport: rt},
	}

	got, err := emb.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || len(got[0]) != 2 {
		t.Fatalf("unexpected embeddings: %v", got)
	}
	if !bytes.Contains(rt.requestBody, []byte(`"model":"nomic-embed-text:latest"`)) {
		t.Fatalf("unexpected request body: %s", string(rt.requestBody))
	}
}

func TestOllamaEmbedder_DimensionMismatch(t *testing.T) {
	emb := &OllamaEmbedder{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text:latest",
		DimVal:  3,
		Client: &http.Client{Transport: &ollamaRecordingRoundTripper{
			statusCode: http.StatusOK,
			body:       `{"embeddings":[[0.1,0.2]]}`,
		}},
	}

	if _, err := emb.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("expected dimension mismatch")
	}
}

func TestDetectOllamaDimension(t *testing.T) {
	rt := &ollamaRecordingRoundTripper{
		statusCode: http.StatusOK,
		body:       `{"embeddings":[[0.1,0.2,0.3]]}`,
	}
	dim, err := DetectOllamaDimension(context.Background(), "http://localhost:11434", "nomic-embed-text:latest", &http.Client{Transport: rt})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dim != 3 {
		t.Fatalf("expected dim 3, got %d", dim)
	}
}

func TestClipForOllamaEmbedding(t *testing.T) {
	input := "abcdefghijklmnopqrstuvwxyz"
	got := clipForOllamaEmbedding(input, 10)
	if got != "abcdefg\n...\nxyz" {
		t.Fatalf("unexpected clipped text: %q", got)
	}
	if clipForOllamaEmbedding("short", 10) != "short" {
		t.Fatal("short text should remain unchanged")
	}
}

type ollamaRecordingRoundTripper struct {
	statusCode  int
	body        string
	requestBody []byte
}

func (r *ollamaRecordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var err error
	r.requestBody, err = io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: r.statusCode,
		Status:     http.StatusText(r.statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(r.body)),
		Request:    req,
	}, nil
}
