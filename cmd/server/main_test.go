package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"personal-search/internal/model"
)

func TestBuildQdrantFilter_SourceAuthorThreadDate(t *testing.T) {
	f := &model.Filters{
		Source:   []string{"chatgpt", "gmail"},
		Author:   "user",
		ThreadID: "thread-1",
		Date: &model.DateRange{
			From: "2024-01-01",
			To:   "2024-01-31",
		},
	}

	got := buildQdrantFilter(f)
	if got == nil {
		t.Fatal("expected filter")
	}
	if len(got.Should) != 2 {
		t.Fatalf("expected 2 source should clauses, got %d", len(got.Should))
	}
	if len(got.Must) != 3 {
		t.Fatalf("expected 3 must clauses, got %d", len(got.Must))
	}
}

func TestBuildQdrantFilter_EmptySourceIgnored(t *testing.T) {
	f := &model.Filters{
		Source: []string{"", "chatgpt"},
	}

	got := buildQdrantFilter(f)
	if got == nil {
		t.Fatal("expected filter")
	}
	if len(got.Should) != 1 {
		t.Fatalf("expected 1 source clause, got %d", len(got.Should))
	}
}

func TestSearchRequestValidate_RejectsKeywords(t *testing.T) {
	req := &model.SearchRequest{
		Query: "test",
		Filters: &model.Filters{
			Keywords: []string{"custody"},
		},
	}

	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRequireAPITokenProtectsNonHealthRoutes(t *testing.T) {
	base := http.NewServeMux()
	base.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	base.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := requireAPIToken(base, "secret-token")

	for _, tc := range []struct {
		name   string
		path   string
		auth   string
		status int
	}{
		{"health without token", "/health", "", http.StatusOK},
		{"search without token", "/search", "", http.StatusUnauthorized},
		{"search wrong token", "/search", "Bearer wrong", http.StatusUnauthorized},
		{"search bearer token", "/search", "Bearer secret-token", http.StatusOK},
		{"search x api key", "/search", "X-API-Key secret-token", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			if strings.HasPrefix(tc.auth, "Bearer ") {
				req.Header.Set("Authorization", tc.auth)
			}
			if strings.HasPrefix(tc.auth, "X-API-Key ") {
				req.Header.Set("X-API-Key", strings.TrimPrefix(tc.auth, "X-API-Key "))
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}
