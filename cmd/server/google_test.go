package main

import (
	"bytes"
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

func TestGoogleHTTPRoutingAndPreservation(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"test:latest","digest":"digest-A"}]}`)
			return
		}
		fmt.Fprint(w, `{"embeddings":[[1,0]]}`)
	}))
	defer ollama.Close()
	googleCalls, chatCalls := 0, 0
	scrollCalls := 0
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":2,"distance":"Cosine"}}}}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/scroll") {
			scrollCalls++
			fmt.Fprint(w, `{"result":{"points":[{"payload":{"source":"google_search","embedding_identity":"ollama:digest-A:clip2000:v1"}}],"next_page_offset":null}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/google_search/") {
			googleCalls++
			var req qdrant.SearchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			raw, _ := json.Marshal(req.Filter)
			if !bytes.Contains(raw, []byte(`"source"`)) || !bytes.Contains(raw, []byte(`"google_search"`)) {
				t.Errorf("filter=%s", raw)
			}
			fmt.Fprint(w, `{"result":[{"id":"point","score":1,"payload":{"doc_id":"event","source":"google_search","text":"known query","timestamp_unix_ms":1767278400123,"account":"personal","url":"https://google.com/search?q=known","provenance":[{"locator":"archive!Search.json","record_index":3,"sha256":"hash"}]}}]}`)
		} else {
			chatCalls++
			fmt.Fprint(w, `{"result":[{"payload":{"source":"chatgpt","doc_id":"chat","text":"chat result"}}]}`)
		}
	}))
	defer store.Close()
	mux := newMux(qdrant.New(store.URL), &embed.FakeEmbedder{DimVal: 2}, "personal_docs", "google_search", ollama.URL, "test:latest")
	for _, source := range []string{"google_search", "chatgpt"} {
		req := httptest.NewRequest("POST", "/search", strings.NewReader(fmt.Sprintf(`{"query":"known query","filters":{"source":[%q],"date":{"from":"2026-01-01","to":"2026-01-01"}}}`, source)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var out model.SearchResponse
		json.Unmarshal(w.Body.Bytes(), &out)
		if len(out.Results) != 1 || out.Results[0].Source != source {
			t.Fatalf("out=%+v", out)
		}
		if source == "google_search" && (out.Results[0].Timestamp != "2026-01-01T14:40:00.123Z" || out.Results[0].Metadata["account"] != "personal" || out.Results[0].Metadata["provenance"] == nil || out.Results[0].Metadata["url"] != "https://google.com/search?q=known") {
			t.Fatalf("lost metadata: %+v", out)
		}
	}
	if googleCalls != 1 || chatCalls != 1 {
		t.Fatalf("google=%d chat=%d", googleCalls, chatCalls)
	}
	req := httptest.NewRequest("POST", "/search", strings.NewReader(`{"query":"known query","filters":{"source":["google_search"]}}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("second google status=%d body=%s", w.Code, w.Body.String())
	}
	if scrollCalls != 1 {
		t.Fatalf("validated google collection %d times, want 1", scrollCalls)
	}
	for _, body := range []string{
		`{"query":"q","filters":{"source":["google_search","all"]}}`,
		`{"query":"q","filters":{"source":["google_search"],"author":"user"}}`,
		`{"query":"q","filters":{"source":["google_search"],"date":{"from":"2026-02-01","to":"2026-01-01"}}}`,
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/search", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("accepted invalid filters: %d %s", w.Code, w.Body.String())
		}
	}
	if googleCalls != 2 || chatCalls != 1 {
		t.Fatal("invalid filters reached search")
	}
}
