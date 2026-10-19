package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrollPreservesLargeIntegerCursor(t *testing.T) {
	const id = "9007199254740993"
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			var body struct {
				Offset json.RawMessage `json:"offset"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if string(body.Offset) != id {
				t.Errorf("cursor=%s, want %s", body.Offset, id)
			}
		}
		fmt.Fprintf(w, `{"result":{"points":[{"id":%s,"payload":{"timestamp_unix_ms":1234567890123}}],"next_page_offset":%s}}`, id, id)
	}))
	defer srv.Close()
	client := New(srv.URL)
	page, err := client.Scroll(context.Background(), "test", ScrollRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page.Result.Points[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != id {
		t.Errorf("point ID=%s, want %s", encoded, id)
	}
	if _, ok := page.Result.Points[0].Payload["timestamp_unix_ms"].(float64); !ok {
		t.Fatal("payload number representation changed")
	}
	if _, err := client.Scroll(context.Background(), "test", ScrollRequest{Limit: 1, Offset: page.Result.NextPageOffset}); err != nil {
		t.Fatal(err)
	}
}
