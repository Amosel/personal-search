package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

func main() {
	var (
		addr       = flag.String("addr", ":8080", "HTTP listen address")
		qdrantURL  = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
		collection = flag.String("collection", "personal_docs", "Qdrant collection name")
		embedder   = flag.String("embedder", "openai", "Embedding provider: openai|fake")
		openaiKey  = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
		modelName  = flag.String("model", "text-embedding-3-large", "OpenAI embedding model")
		dim        = flag.Int("dim", 3072, "Embedding dimension")
	)
	flag.Parse()

	if *collection == "" {
		fatal("--collection is required")
	}
	emb, err := buildEmbedder(*embedder, *openaiKey, *modelName, *dim)
	if err != nil {
		fatal(err.Error())
	}

	qc := qdrant.New(*qdrantURL)
	if err := qc.EnsureCollection(context.Background(), *collection, emb.Dim()); err != nil {
		fatal(fmt.Sprintf("ensure collection: %v", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := qc.Healthy(r.Context()); err != nil {
			http.Error(w, "qdrant unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req model.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := req.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Limit <= 0 {
			req.Limit = 20
		}

		vecs, err := emb.Embed(r.Context(), []string{req.Query})
		if err != nil {
			http.Error(w, "embedding error", http.StatusInternalServerError)
			return
		}

		out, err := qc.Search(r.Context(), *collection, qdrant.SearchRequest{
			Vector:      vecs[0],
			Limit:       req.Limit,
			WithPayload: true,
			Filter:      buildQdrantFilter(req.Filters),
		})
		if err != nil {
			http.Error(w, "search error", http.StatusInternalServerError)
			return
		}

		resp := model.SearchResponse{Results: make([]model.SearchResult, 0, len(out.Result))}
		for _, hit := range out.Result {
			payload := hit.Payload
			id := payloadString(payload, "doc_id")
			if id == "" {
				id = pointIDToString(hit.ID)
			}

			ts := ""
			if ms, ok := payloadFloat(payload, "timestamp_unix_ms"); ok {
				ts = time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
			}

			resp.Results = append(resp.Results, model.SearchResult{
				ID:        id,
				Score:     hit.Score,
				Text:      payloadString(payload, "text"),
				Timestamp: ts,
				Source:    payloadString(payload, "source"),
				Metadata:  payload,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	if err := http.ListenAndServe(*addr, mux); err != nil {
		fatal(err.Error())
	}
}

func buildEmbedder(kind, key, modelName string, dim int) (embed.Embedder, error) {
	switch kind {
	case "openai":
		if key == "" {
			return nil, fmt.Errorf("missing OpenAI key (set OPENAI_API_KEY or --openai_key)")
		}
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &embed.OpenAIEmbedder{APIKey: key, Model: modelName, DimVal: dim}, nil
	case "fake":
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &embed.FakeEmbedder{DimVal: dim}, nil
	default:
		return nil, fmt.Errorf("unsupported --embedder value: %s", kind)
	}
}

func buildQdrantFilter(f *model.Filters) *qdrant.Filter {
	if f == nil {
		return nil
	}

	must := make([]any, 0, 4)

	if len(f.Source) > 0 && f.Source[0] != "" {
		must = append(must, qdrant.FieldCondition{
			Key:   "source",
			Match: &qdrant.MatchValue{Value: f.Source[0]},
		})
	}
	if f.Author != "" {
		must = append(must, qdrant.FieldCondition{
			Key:   "author",
			Match: &qdrant.MatchValue{Value: f.Author},
		})
	}
	if f.ThreadID != "" {
		must = append(must, qdrant.FieldCondition{
			Key:   "thread_id",
			Match: &qdrant.MatchValue{Value: f.ThreadID},
		})
	}
	if f.Date != nil && (f.Date.From != "" || f.Date.To != "") {
		rng := qdrant.Range{}
		if f.Date.From != "" {
			t, err := time.Parse("2006-01-02", f.Date.From)
			if err == nil {
				v := float64(t.UTC().UnixMilli())
				rng.Gte = &v
			}
		}
		if f.Date.To != "" {
			t, err := time.Parse("2006-01-02", f.Date.To)
			if err == nil {
				end := t.Add(24*time.Hour - time.Millisecond).UTC()
				v := float64(end.UnixMilli())
				rng.Lte = &v
			}
		}
		must = append(must, qdrant.FieldCondition{
			Key:   "timestamp_unix_ms",
			Range: &rng,
		})
	}

	if len(must) == 0 {
		return nil
	}
	return &qdrant.Filter{Must: must}
}

func payloadString(payload map[string]any, key string) string {
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func payloadFloat(payload map[string]any, key string) (float64, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

func pointIDToString(v any) string {
	switch id := v.(type) {
	case string:
		return id
	case float64:
		return strconv.FormatUint(uint64(id), 10)
	case int:
		return strconv.Itoa(id)
	default:
		return fmt.Sprintf("%v", id)
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
