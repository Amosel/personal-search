package qdrant

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		size                       int
		distance, identity, source string
		bad                        bool
	}{
		{"compatible", 8, "Cosine", "model-A", "google_search", false},
		{"same dimension different model", 8, "Cosine", "model-B", "google_search", true},
		{"unmarked", 8, "Cosine", "", "google_search", true},
		{"foreign source", 8, "Cosine", "model-A", "chatgpt", true},
		{"wrong dimension", 9, "Cosine", "model-A", "google_search", true},
		{"wrong distance", 8, "Dot", "model-A", "google_search", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					fmt.Fprintf(w, `{"result":{"config":{"params":{"vectors":{"size":%d,"distance":%q}}}}}`, tc.size, tc.distance)
					return
				}
				fmt.Fprintf(w, `{"result":{"points":[{"payload":{"embedding_identity":%q,"source":%q}}],"next_page_offset":null}}`, tc.identity, tc.source)
			}))
			defer srv.Close()
			err := New(srv.URL).ValidateEmbedding(context.Background(), "google_search", "model-A", 8)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestEmbeddingChecksEveryPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":8,"distance":"Cosine"}}}}}`)
			return
		}
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"result":{"points":[{"payload":{"embedding_identity":"model-A","source":"google_search"}}],"next_page_offset":2}}`)
			return
		}
		fmt.Fprint(w, `{"result":{"points":[{"payload":{"embedding_identity":"model-B","source":"google_search"}}],"next_page_offset":null}}`)
	}))
	defer srv.Close()
	if err := New(srv.URL).ValidateEmbedding(context.Background(), "google_search", "model-A", 8); err == nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
func TestUpsertRequiresCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"result":{"status":"acknowledged"}}`) }))
	defer srv.Close()
	err := New(srv.URL).Upsert(context.Background(), "google_search", []Point{{ID: NumericID(1), Vector: []float32{1}}})
	if err == nil || !strings.Contains(err.Error(), "unconfirmed") {
		t.Fatalf("err=%v", err)
	}
}
