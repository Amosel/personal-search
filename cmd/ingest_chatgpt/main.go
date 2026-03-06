package main

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"os"

	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
)

func main() {
	var (
		exportPath = flag.String("export", "", "Path to ChatGPT export JSON or ZIP")
		qdrantURL  = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
		collection = flag.String("collection", "personal_docs", "Qdrant collection name")
		embedder   = flag.String("embedder", "openai", "Embedding provider: openai|fake")
		openaiKey  = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
		modelName  = flag.String("model", "text-embedding-3-large", "OpenAI embedding model")
		dim        = flag.Int("dim", 3072, "Embedding dimension")
		batchSize  = flag.Int("batch", 64, "Embedding batch size")
		maxDocs    = flag.Int("max_docs", 0, "Optional cap on number of documents to ingest (0 = all)")
	)
	flag.Parse()

	if *exportPath == "" {
		fatal("--export is required")
	}
	if *collection == "" {
		fatal("--collection is required")
	}
	if *batchSize <= 0 {
		fatal("--batch must be > 0")
	}
	if *maxDocs < 0 {
		fatal("--max_docs must be >= 0")
	}

	emb, err := buildEmbedder(*embedder, *openaiKey, *modelName, *dim)
	if err != nil {
		fatal(err.Error())
	}

	ctx := context.Background()
	exp, err := chatgpt.LoadExport(*exportPath)
	if err != nil {
		fatal(fmt.Sprintf("load export: %v", err))
	}

	docs, err := chatgpt.ToDocuments(exp)
	if err != nil {
		fatal(fmt.Sprintf("convert export: %v", err))
	}
	if len(docs) == 0 {
		fatal("no documents produced from export")
	}
	if *maxDocs > 0 && len(docs) > *maxDocs {
		docs = docs[:*maxDocs]
	}

	qc := qdrant.New(*qdrantURL)
	if err := qc.EnsureCollection(ctx, *collection, emb.Dim()); err != nil {
		fatal(fmt.Sprintf("ensure collection: %v", err))
	}

	for i := 0; i < len(docs); i += *batchSize {
		j := i + *batchSize
		if j > len(docs) {
			j = len(docs)
		}
		batch := docs[i:j]

		texts := make([]string, len(batch))
		for k := range batch {
			texts[k] = batch[k].Text
		}

		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			fatal(fmt.Sprintf("embed batch [%d:%d]: %v", i, j, err))
		}

		points := make([]qdrant.Point, 0, len(batch))
		for k, d := range batch {
			payload := map[string]any{
				"doc_id":            d.ID,
				"source":            d.Source,
				"type":              d.Type,
				"timestamp_unix_ms": float64(d.Timestamp),
				"text":              d.Text,
			}
			for mk, mv := range d.Metadata {
				payload[mk] = mv
			}

			points = append(points, qdrant.Point{
				ID:      pointIDFromDocID(d.ID),
				Vector:  vecs[k],
				Payload: payload,
			})
		}

		if err := qc.Upsert(ctx, *collection, points); err != nil {
			fatal(fmt.Sprintf("upsert batch [%d:%d]: %v", i, j, err))
		}

		fmt.Printf("upserted %d/%d\n", j, len(docs))
	}

	fmt.Printf("ingestion complete: %d documents into %s\n", len(docs), *collection)
}

func buildEmbedder(kind, key, model string, dim int) (embed.Embedder, error) {
	switch kind {
	case "openai":
		if key == "" {
			return nil, fmt.Errorf("missing OpenAI key (set OPENAI_API_KEY or --openai_key)")
		}
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &embed.OpenAIEmbedder{
			APIKey: key,
			Model:  model,
			DimVal: dim,
		}, nil
	case "fake":
		if dim <= 0 {
			return nil, fmt.Errorf("--dim must be > 0")
		}
		return &embed.FakeEmbedder{DimVal: dim}, nil
	default:
		return nil, fmt.Errorf("unsupported --embedder value: %s", kind)
	}
}

func pointIDFromDocID(docID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	return h.Sum64()
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
