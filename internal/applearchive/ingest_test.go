package applearchive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strings"
	"testing"
)

type identifiedFake struct{ embed.FakeEmbedder }

func (e identifiedFake) EmbeddingIdentity() string { return "fixture-identity" }

func TestRunUpsertsStableIDsAndGuardsCollections(t *testing.T) {
	var writes [][]qdrant.Point
	foreign := false
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":2,"distance":"Cosine"}}}}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/scroll") {
			if foreign {
				fmt.Fprint(w, `{"result":{"points":[{"payload":{"source":"google_search","embedding_identity":"fixture-identity"}}]}}`)
			} else {
				fmt.Fprint(w, `{"result":{"points":[]}}`)
			}
			return
		}
		var body struct {
			Points []qdrant.Point `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		writes = append(writes, body.Points)
		fmt.Fprint(w, `{"result":{"status":"completed"}}`)
	}))
	defer store.Close()
	emb := &identifiedFake{embed.FakeEmbedder{DimVal: 2}}
	docs := []model.Document{{ID: "stable-id", Source: "apple_notes", Type: "note", Timestamp: 1767225600000, Text: "first", Metadata: map[string]any{"native_id": "note-1", "title": "Original"}}}
	run := func() (int, error) {
		return Run(context.Background(), docs, "apple_notes", "notes", 8, emb, qdrant.New(store.URL))
	}
	if n, err := run(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	docs[0].Text = "edited"
	docs[0].Metadata["title"] = "Renamed"
	if n, err := run(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(writes) != 2 || writes[0][0].ID != writes[1][0].ID || writes[1][0].Payload["text"] != "edited" || writes[1][0].Payload["embedding_identity"] != "fixture-identity" {
		t.Fatalf("writes=%+v", writes)
	}
	foreign = true
	if n, err := run(); err == nil || n != 0 || len(writes) != 2 {
		t.Fatalf("foreign collection accepted: n=%d err=%v", n, err)
	}
}

func TestLoadExportThroughPython(t *testing.T) {
	// Exercise the embedded script, not just its standalone unit tests.
	_, err := Load(context.Background(), "apple_notes", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no JSON exports") {
		t.Fatalf("err=%v", err)
	}
}

func TestSourceExporterFixtures(t *testing.T) {
	out, err := exec.Command("python3", "-B", "-m", "unittest", "discover", "-s", ".", "-p", "*_test.py").CombinedOutput()
	if err != nil {
		t.Fatalf("Python exporter tests: %v\n%s", err, out)
	}
}

type malformedEmbedder struct {
	identifiedFake
	vectors [][]float32
}

func (e malformedEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return e.vectors, nil
}
func TestMalformedVectorsStopBeforeAppleStorage(t *testing.T) {
	for _, source := range []string{"apple_notes", "voicememo"} {
		for _, vectors := range [][][]float32{nil, {{1}}, {{1, float32(math.NaN())}}, {{1, float32(math.Inf(1))}}} {
			t.Run(fmt.Sprint(source, vectors), func(t *testing.T) {
				writes := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":2,"distance":"Cosine"}}}}}`)
						return
					}
					if strings.HasSuffix(r.URL.Path, "/scroll") {
						fmt.Fprint(w, `{"result":{"points":[]}}`)
						return
					}
					writes++
					fmt.Fprint(w, `{"result":{"status":"completed"}}`)
				}))
				defer server.Close()
				kind := "note"
				if source == "voicememo" {
					kind = "transcript"
				}
				docs := []model.Document{{ID: "synthetic", Source: source, Type: kind, Timestamp: 1, Text: "example"}}
				e := malformedEmbedder{identifiedFake: identifiedFake{embed.FakeEmbedder{DimVal: 2}}, vectors: vectors}
				n, err := Run(context.Background(), docs, source, "synthetic", 1, &e, qdrant.New(server.URL))
				var boundary *embed.VectorError
				if n != 0 || writes != 0 || !errors.As(err, &boundary) {
					t.Fatalf("n=%d writes=%d err=%v", n, writes, err)
				}
			})
		}
	}
}
