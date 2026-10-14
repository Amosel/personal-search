package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strings"
	"testing"
)

func TestMergedSourcesRoutingRankingAndFailure(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"test:latest","digest":"A"}]}`)
			return
		}
		fmt.Fprint(w, `{"embeddings":[[1,0]]}`)
	}))
	defer ollama.Close()
	fail := false
	calls := map[string]int{}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := strings.Split(r.URL.Path, "/")[2]
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":2,"distance":"Cosine"}}}}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/scroll") {
			fmt.Fprintf(w, `{"result":{"points":[{"payload":{"source":%q,"embedding_identity":"ollama:A:clip2000:v1"}}]}}`, source)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/count") {
			fmt.Fprint(w, `{"result":{"count":2}}`)
			return
		}
		if fail && source == "voicememo" {
			http.Error(w, "offline", 503)
			return
		}
		calls[source]++
		var req qdrant.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		filter, _ := json.Marshal(req.Filter)
		if !strings.Contains(string(filter), source) || !strings.Contains(string(filter), "timestamp_unix_ms") {
			t.Errorf("filter missing source/date: %s", filter)
		}
		fmt.Fprintf(w, `{"result":[{"score":0.1,"payload":{"doc_id":"same","source":%q,"text":"first"}},{"score":0.01,"payload":{"doc_id":"second","source":%q,"text":"second"}}]}`, source, source)
	}))
	defer store.Close()
	mux := newMux(qdrant.New(store.URL), &embed.FakeEmbedder{DimVal: 2}, "chatgpt", "google_search", ollama.URL, "test:latest")
	body := `{"query":"q","limit":4,"filters":{"source":["all"],"date":{"from":"2026-01-01"}}}`
	request := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/search", strings.NewReader(body)))
		return w
	}
	w := request(body)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var response model.SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Ranking != "reciprocal_rank_k60" || len(response.Results) != 4 || len(calls) != 4 {
		t.Fatalf("response=%+v calls=%v", response, calls)
	}
	for i, source := range []string{"chatgpt", "google_search", "apple_notes", "voicememo"} {
		hit := response.Results[i]
		if hit.Source != source || hit.ID != "same" || hit.Metadata["source_score"] != 0.1 || hit.Score != 1.0/61 {
			t.Fatalf("hit=%+v", hit)
		}
	}
	// A partial backend failure must not masquerade as complete results.
	fail = true
	if w := request(body); w.Code != 503 || !strings.Contains(w.Body.String(), "voicememo") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, source := range []string{`["apple_notes","apple_notes"]`, `["all","chatgpt"]`} {
		if w := request(`{"query":"q","filters":{"source":` + source + `}}`); w.Code != 400 {
			t.Fatalf("accepted %s", source)
		}
	}
	w = request(`{"query":"q","filters":{"source":["apple_notes","voicememo"],"author":"user"}}`)
	if w.Code != 400 {
		t.Fatal("accepted mixed author filter")
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/status?source=all", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"points":8`) {
		t.Fatalf("status: %s", w.Body.String())
	}
}
