package main

import (
	"context"
	embedfs "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

//go:embed web
var webFiles embedfs.FS

func main() {
	var (
		addr        = flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
		qdrantURL   = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
		collection  = flag.String("collection", "personal_docs", "Qdrant collection name")
		embedder    = flag.String("embedder", "openai", "Embedding provider: openai|fake|ollama")
		openaiKey   = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
		modelName   = flag.String("model", embed.DefaultOpenAIModel, "OpenAI embedding model")
		ollamaURL   = flag.String("ollama_url", embed.DefaultOllamaURL, "Ollama base URL")
		ollamaModel = flag.String("ollama_model", embed.DefaultOllamaModel, "Ollama embedding model")
		dim         = flag.Int("dim", 0, "Embedding dimension (0 = infer for Ollama)")
	)
	flag.Parse()

	if *collection == "" {
		fatal("--collection is required")
	}
	emb, err := embed.New(*embedder, *openaiKey, *modelName, *ollamaURL, *ollamaModel, *dim)
	if err != nil {
		fatal(err.Error())
	}

	qc := qdrant.New(*qdrantURL)
	if err := qc.EnsureCollection(context.Background(), *collection, emb.Dim()); err != nil {
		fatal(fmt.Sprintf("ensure collection: %v", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || (r.URL.Path != "/" && r.URL.Path != "/index.html") {
			http.NotFound(w, r)
			return
		}
		page, err := webFiles.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "UI unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		count, err := qc.Count(r.Context(), *collection)
		if err != nil {
			http.Error(w, "index unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "collection": *collection, "points": count})
	})
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
			resp.Results = append(resp.Results, toSearchResult(hit))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/thread", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
		if threadID == "" {
			http.Error(w, "thread_id is required", http.StatusBadRequest)
			return
		}

		const pageSize, maxMessages = 500, 5000
		filter := &qdrant.Filter{Must: []any{qdrant.FieldCondition{
			Key: "thread_id", Match: &qdrant.MatchValue{Value: threadID},
		}}}
		messages := make([]model.SearchResult, 0, pageSize)
		var offset any
		truncated := false
		for len(messages) < maxMessages {
			limit := pageSize
			if remaining := maxMessages - len(messages); remaining < limit {
				limit = remaining
			}
			page, err := qc.Scroll(r.Context(), *collection, qdrant.ScrollRequest{
				Filter: filter, Limit: limit, Offset: offset, WithPayload: true,
			})
			if err != nil {
				http.Error(w, "thread lookup error", http.StatusInternalServerError)
				return
			}
			for _, hit := range page.Result.Points {
				messages = append(messages, toSearchResult(hit))
			}
			offset = page.Result.NextPageOffset
			if offset == nil || len(page.Result.Points) == 0 {
				break
			}
			if len(messages) >= maxMessages {
				truncated = true
			}
		}
		sort.SliceStable(messages, func(i, j int) bool {
			return messages[i].Timestamp < messages[j].Timestamp
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": messages, "truncated": truncated})
	})

	if err := http.ListenAndServe(*addr, mux); err != nil {
		fatal(err.Error())
	}
}

func toSearchResult(hit qdrant.ScoredPoint) model.SearchResult {
	payload := hit.Payload
	id := payloadString(payload, "doc_id")
	if id == "" {
		id = pointIDToString(hit.ID)
	}
	ts := ""
	if ms, ok := payloadFloat(payload, "timestamp_unix_ms"); ok {
		ts = time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
	}
	return model.SearchResult{
		ID: id, Score: hit.Score, Text: payloadString(payload, "text"),
		Timestamp: ts, Source: payloadString(payload, "source"), Metadata: payload,
	}
}

func buildQdrantFilter(f *model.Filters) *qdrant.Filter {
	if f == nil {
		return nil
	}

	must := make([]any, 0, 4)
	should := make([]any, 0, 2)

	for _, src := range f.Source {
		if src == "" {
			continue
		}
		should = append(should, qdrant.FieldCondition{
			Key:   "source",
			Match: &qdrant.MatchValue{Value: src},
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
		if len(should) == 0 {
			return nil
		}
		return &qdrant.Filter{Should: should}
	}
	if len(should) == 0 {
		return &qdrant.Filter{Must: must}
	}
	return &qdrant.Filter{
		Must:   must,
		Should: should,
	}
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
