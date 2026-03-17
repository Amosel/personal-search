package chatgptsearch

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestHTTPClientSearch_Success(t *testing.T) {
	rt := &stubRoundTripper{
		statusCode: http.StatusOK,
		body:       `{"results":[{"id":"1","score":0.9,"text":"match","timestamp":"2024-12-31T00:00:00Z","source":"chatgpt","metadata":{"author":"user"}}]}`,
	}
	c := &HTTPClient{
		BaseURL: "http://example.test",
		Client:  &http.Client{Transport: rt},
	}

	out, err := c.Search(context.Background(), BuildRequest(Options{Query: "match"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Source != "chatgpt" {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
	if !bytes.Contains(rt.requestBody, []byte(`"source":["chatgpt"]`)) {
		t.Fatalf("expected request to scope source to chatgpt, got %s", string(rt.requestBody))
	}
}

func TestHTTPClientSearch_NonOK(t *testing.T) {
	c := &HTTPClient{
		BaseURL: "http://example.test",
		Client: &http.Client{Transport: &stubRoundTripper{
			statusCode: http.StatusBadRequest,
			body:       `keywords filter is not supported`,
		}},
	}

	_, err := c.Search(context.Background(), BuildRequest(Options{Query: "match"}))
	if err == nil {
		t.Fatal("expected error")
	}
}

type stubRoundTripper struct {
	statusCode  int
	body        string
	requestBody []byte
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	s.requestBody = body
	return &http.Response{
		StatusCode: s.statusCode,
		Status:     http.StatusText(s.statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Request:    req,
	}, nil
}
