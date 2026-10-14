package applearchive

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"personal-search/internal/embed"
	"personal-search/internal/googlesearch"
	"personal-search/internal/qdrant"
)

func Main(source string) {
	input := flag.String("input", "", "Notes MCP JSON file/page directory or VoiceMemo robot SQLite DB")
	collection := flag.String("collection", source, "Dedicated collection")
	endpoint := flag.String("qdrant", "http://localhost:6333", "Qdrant URL")
	ollama := flag.String("ollama_url", embed.DefaultOllamaURL, "Ollama URL")
	model := flag.String("ollama_model", embed.DefaultOllamaModel, "Embedding model; must match server")
	batch := flag.Int("batch", 8, "Batch size")
	maxDocs := flag.Int("max_docs", 0, "Bound import; zero imports all")
	dry := flag.Bool("dry-run", false, "Read and validate without contacting embedding/index services")
	flag.Parse()
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if *batch <= 0 || *maxDocs < 0 || *collection == "" {
		fail(fmt.Errorf("positive batch, nonnegative max_docs and collection required"))
	}
	ctx := context.Background()
	archive, err := Load(ctx, source, *input)
	if err != nil {
		fail(err)
	}
	docs := archive.Documents
	if *maxDocs > 0 && len(docs) > *maxDocs {
		docs = docs[:*maxDocs]
	}
	archive.Report["selected"] = len(docs)
	archive.Report["dry_run"] = *dry
	if !*dry && len(docs) > 0 {
		emb, e := googlesearch.NewEmbedder(ctx, *ollama, *model)
		if e != nil {
			fail(e)
		}
		n, e := Run(ctx, docs, source, *collection, *batch, emb, qdrant.New(*endpoint))
		archive.Report["acknowledged"] = n
		if e != nil {
			json.NewEncoder(os.Stdout).Encode(archive.Report)
			fail(e)
		}
	}
	json.NewEncoder(os.Stdout).Encode(archive.Report)
}
