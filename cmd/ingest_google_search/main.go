package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"personal-search/internal/embed"
	"personal-search/internal/googlesearch"
	"personal-search/internal/qdrant"
)

func main() {
	input := flag.String("archive", "", "Normalized crawler archive")
	collection := flag.String("collection", "google_search", "Dedicated Qdrant collection")
	endpoint := flag.String("qdrant", "http://localhost:6333", "Qdrant URL")
	ollama := flag.String("ollama_url", embed.DefaultOllamaURL, "Local Ollama URL")
	model := flag.String("ollama_model", embed.DefaultOllamaModel, "Embedding model; must match search server")
	maxDocs := flag.Int("max_docs", 0, "Bound import to first N records; 0 imports all")
	batch := flag.Int("batch", 8, "Batch size")
	dry := flag.Bool("dry-run", false, "Validate archive without service calls")
	flag.Parse()
	if *maxDocs < 0 || *batch <= 0 || *collection == "" {
		fail(fmt.Errorf("nonnegative max_docs, positive batch and collection required"))
	}
	docs, err := googlesearch.Load(*input)
	if err != nil {
		fail(err)
	}
	if *dry {
		fmt.Printf("validated=%d\n", len(docs))
		return
	}
	total := len(docs)
	if *maxDocs > 0 && len(docs) > *maxDocs {
		docs = docs[:*maxDocs]
	}
	emb, err := googlesearch.NewEmbedder(context.Background(), *ollama, *model)
	if err != nil {
		fmt.Printf("acknowledged=0 selected=%d archive_total=%d collection=%s\n", len(docs), total, *collection)
		fail(err)
	}
	n, err := googlesearch.Run(context.Background(), docs, *collection, *batch, emb, qdrant.New(*endpoint))
	fmt.Printf("acknowledged=%d selected=%d archive_total=%d collection=%s\n", n, len(docs), total, *collection)
	if err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
