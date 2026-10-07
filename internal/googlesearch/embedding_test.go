package googlesearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPinnedModelChange(t *testing.T) {
	digest := "A"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprintf(w, `{"models":[{"name":"test:latest","digest":%q}]}`, digest)
			return
		}
		fmt.Fprint(w, `{"embeddings":[[1,0]]}`)
	}))
	defer srv.Close()
	e, err := NewEmbedder(context.Background(), srv.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(context.Background(), []string{"query"}); err != nil {
		t.Fatal(err)
	}
	digest = "B"
	if _, err = e.Embed(context.Background(), []string{"query"}); err == nil {
		t.Fatal("accepted model change with identical dimension")
	}
}
