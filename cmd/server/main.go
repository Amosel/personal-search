package main

import (
	"context"
	"crypto/subtle"
	embedfs "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"personal-search/internal/embed"
	"personal-search/internal/googlesearch"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

//go:embed web
var webFiles embedfs.FS

func main() {
	var (
		addr             = flag.String("addr", defaultAddr(), "HTTP listen address")
		qdrantURL        = flag.String("qdrant", envDefault("QDRANT_URL", "http://localhost:6333"), "Qdrant base URL")
		googleCollection = flag.String("google_collection", envDefault("GOOGLE_COLLECTION", "google_search"), "Dedicated Google Search collection")
		googleModel      = flag.String("google_ollama_model", envDefault("GOOGLE_OLLAMA_MODEL", embed.DefaultOllamaModel), "Google embedding model")
		googleURL        = flag.String("google_ollama_url", envDefault("GOOGLE_OLLAMA_URL", embed.DefaultOllamaURL), "Google Ollama URL")
		collection       = flag.String("collection", envDefault("COLLECTION", "personal_docs"), "Qdrant collection name")
		embedder         = flag.String("embedder", envDefault("EMBEDDER", "openai"), "Embedding provider: openai|fake|ollama")
		openaiKey        = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
		modelName        = flag.String("model", envDefault("OPENAI_MODEL", embed.DefaultOpenAIModel), "OpenAI embedding model")
		ollamaURL        = flag.String("ollama_url", envDefault("OLLAMA_URL", embed.DefaultOllamaURL), "Ollama base URL")
		ollamaModel      = flag.String("ollama_model", envDefault("OLLAMA_MODEL", embed.DefaultOllamaModel), "Ollama embedding model")
		dim              = flag.Int("dim", envInt("DIM", 0), "Embedding dimension (0 = infer for Ollama)")
		notesCollection  = flag.String("notes_collection", envDefault("NOTES_COLLECTION", "apple_notes"), "Apple Notes collection")
		voiceCollection  = flag.String("voice_collection", envDefault("VOICE_COLLECTION", "voicememo"), "VoiceMemo collection")
		appleURL         = flag.String("apple_ollama_url", envDefault("APPLE_OLLAMA_URL", embed.DefaultOllamaURL), "Apple sources Ollama URL")
		appleModel       = flag.String("apple_ollama_model", envDefault("APPLE_OLLAMA_MODEL", embed.DefaultOllamaModel), "Apple sources embedding model")
		apiToken         = flag.String("api_token", os.Getenv("PERSONAL_SEARCH_API_TOKEN"), "Bearer token required for non-health HTTP routes")
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

	handler := secureResponseHeaders(requireAPIToken(newMux(qc, emb, *collection, *googleCollection, *googleURL, *googleModel, appleConfig{*notesCollection, *voiceCollection, *appleURL, *appleModel}), *apiToken))

	if err := http.ListenAndServe(*addr, handler); err != nil {
		fatal(err.Error())
	}
}

func secureResponseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func newMux(qc *qdrant.Client, emb embed.Embedder, collection, googleCollection, googleURL, googleModel string, configs ...appleConfig) *http.ServeMux {
	mux := http.NewServeMux()
	config := appleConfig{"apple_notes", "voicememo", googleURL, googleModel}
	if len(configs) > 0 {
		config = configs[0]
	}
	collections := map[string]string{"chatgpt": collection, "google_search": googleCollection, "apple_notes": config.notes, "voicememo": config.voice}
	runtimes := map[string]*validatedSourceRuntime{}
	for source, name := range collections {
		if source == "chatgpt" || source == "google_search" {
			continue
		}
		runtimes[source] = &validatedSourceRuntime{qc: qc, collection: name, baseURL: config.url, model: config.model, source: source}
	}
	googleRuntime := &validatedSourceRuntime{
		qc:         qc,
		collection: googleCollection,
		baseURL:    googleURL,
		model:      googleModel,
	}
	runtimes["google_search"] = googleRuntime
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
		source := r.URL.Query().Get("source")
		sources, err := searchSources(&model.Filters{Source: []string{source}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var count int64
		for _, source := range sources {
			name := collections[source]
			if name == "" {
				if runtimes[source] != nil {
					http.Error(w, source+" collection not configured", http.StatusBadRequest)
					return
				}
				name = collection
			}
			n, err := qc.Count(r.Context(), name)
			if err != nil {
				http.Error(w, source+" index unavailable", http.StatusServiceUnavailable)
				return
			}
			count += int64(n)
		}
		statusCollection := collections[sources[0]]
		if statusCollection == "" {
			statusCollection = collection
		}
		if len(sources) > 1 {
			statusCollection = "all"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "collection": statusCollection, "points": count})
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

		sources, err := searchSources(req.Filters)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Limit > 1000 {
			http.Error(w, "limit must be <= 1000", http.StatusBadRequest)
			return
		}
		resp := model.SearchResponse{Results: []model.SearchResult{}}
		for _, source := range sources {
			searchCollection := collections[source]
			if searchCollection == "" && runtimes[source] == nil {
				searchCollection = collection
			}
			queryEmbedder := emb
			if runtimes[source] != nil {
				for other, name := range collections {
					if name == searchCollection && other != source {
						http.Error(w, "sources require separate collections", http.StatusBadRequest)
						return
					}
				}
				if searchCollection == "" {
					http.Error(w, source+" collection not configured", http.StatusBadRequest)
					return
				}
				sourceEmb, err := runtimes[source].embedder(r.Context())
				if err != nil {
					http.Error(w, source+": "+err.Error(), http.StatusServiceUnavailable)
					return
				}
				queryEmbedder = sourceEmb
			}
			vecs, err := queryEmbedder.Embed(r.Context(), []string{req.Query})
			if err != nil || len(vecs) != 1 {
				http.Error(w, source+" embedding error", http.StatusServiceUnavailable)
				return
			}
			filters := model.Filters{}
			if req.Filters != nil {
				filters = *req.Filters
			}
			filters.Source = []string{source}
			out, err := qc.Search(r.Context(), searchCollection, qdrant.SearchRequest{Vector: vecs[0], Limit: req.Limit, WithPayload: true, Filter: buildQdrantFilter(&filters)})
			if err != nil {
				http.Error(w, source+" search error", http.StatusServiceUnavailable)
				return
			}
			for rank, hit := range out.Result {
				result := toSearchResult(hit)
				if len(sources) > 1 {
					if result.Metadata == nil {
						result.Metadata = map[string]any{}
					}
					result.Metadata["source_score"] = result.Score
					result.Metadata["source_rank"] = rank + 1
					result.Score = 1.0 / float64(60+rank+1)
				}
				resp.Results = append(resp.Results, result)
			}
		}
		if len(sources) > 1 {
			resp.Ranking = "reciprocal_rank_k60"
			sort.SliceStable(resp.Results, func(i, j int) bool { return resp.Results[i].Score > resp.Results[j].Score })
		}
		if len(resp.Results) > req.Limit {
			resp.Results = resp.Results[:req.Limit]
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
		}, qdrant.FieldCondition{Key: "source", Match: &qdrant.MatchValue{Value: "chatgpt"}}}}
		messages := make([]model.SearchResult, 0, pageSize)
		var offset any
		truncated := false
		for len(messages) < maxMessages {
			limit := pageSize
			if remaining := maxMessages - len(messages); remaining < limit {
				limit = remaining
			}
			page, err := qc.Scroll(r.Context(), collection, qdrant.ScrollRequest{
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
			left, _ := time.Parse(time.RFC3339Nano, messages[i].Timestamp)
			right, _ := time.Parse(time.RFC3339Nano, messages[j].Timestamp)
			return left.Before(right)
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": messages, "truncated": truncated})
	})

	return mux
}

type appleConfig struct{ notes, voice, url, model string }

type validatedSourceRuntime struct {
	source     string
	mu         sync.Mutex
	qc         *qdrant.Client
	collection string
	baseURL    string
	model      string
	cached     *googlesearch.PinnedEmbedder
}

func (r *validatedSourceRuntime) embedder(ctx context.Context) (*googlesearch.PinnedEmbedder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil {
		return r.cached, nil
	}
	emb, err := googlesearch.NewEmbedder(ctx, r.baseURL, r.model)
	if err != nil {
		return nil, err
	}
	source := r.source
	if source == "" {
		source = "google_search"
	}
	if err := r.qc.ValidateSourceEmbedding(ctx, r.collection, emb.EmbeddingIdentity(), emb.Dim(), source); err != nil {
		return nil, err
	}
	r.cached = emb
	return emb, nil
}

func toSearchResult(hit qdrant.ScoredPoint) model.SearchResult {
	payload := hit.Payload
	id := payloadString(payload, "doc_id")
	if id == "" {
		id = pointIDToString(hit.ID)
	}
	ts := ""
	if ms, ok := payloadFloat(payload, "timestamp_unix_ms"); ok {
		ts = time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339Nano)
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

func requireAPIToken(next http.Handler, token string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if hasAPIToken(r, token) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="personal-search"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func hasAPIToken(r *http.Request, token string) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") && constantTimeEqual(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")), token) {
		return true
	}
	return constantTimeEqual(strings.TrimSpace(r.Header.Get("X-API-Key")), token)
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func defaultAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return envDefault("ADDR", "127.0.0.1:8080")
}
