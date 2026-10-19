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

func TestThreadPreservesLargeIDsAndPagination(t *testing.T) {
	for _, mode := range []string{"complete", "repeated", "empty", "cycle"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct {
					Offset json.RawMessage `json:"offset"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if calls == 2 && string(req.Offset) != "9007199254740993" {
					t.Errorf("offset=%s", req.Offset)
				}
				if mode == "empty" {
					fmt.Fprint(w, `{"result":{"points":[],"next_page_offset":0}}`)
					return
				}
				next := "9007199254740993"
				if mode == "complete" && calls == 2 {
					next = "null"
				}
				if mode == "cycle" && calls == 2 {
					next = "18446744073709551615"
				}
				fmt.Fprintf(w, `{"result":{"points":[{"id":%s,"payload":{"source":"chatgpt","thread_id":"t","text":"message %d","timestamp_unix_ms":1234567890123}}],"next_page_offset":%s}}`, []string{"9007199254740993", "18446744073709551615"}[(calls-1)%2], calls, next)
			}))
			defer store.Close()
			mux := newMux(qdrant.New(store.URL), &embed.FakeEmbedder{DimVal: 3}, "chat", "google", "", "")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/thread?thread_id=t", nil))
			if mode != "complete" {
				if w.Code != http.StatusBadGateway || calls > 3 {
					t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
				}
				return
			}
			if w.Code != 200 || calls != 2 {
				t.Fatalf("status=%d calls=%d", w.Code, calls)
			}
			var response struct {
				Results   []model.SearchResult `json:"results"`
				Truncated bool                 `json:"truncated"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Results) != 2 || response.Truncated {
				t.Fatalf("response=%+v", response)
			}
			if response.Results[0].ID != "9007199254740993" || response.Results[1].ID != "18446744073709551615" {
				t.Fatalf("IDs=%+v", response.Results)
			}
			if !strings.HasSuffix(response.Results[0].Timestamp, ".123Z") {
				t.Fatalf("timestamp=%s", response.Results[0].Timestamp)
			}
		})
	}
}
