package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"personal-search/internal/chatgpt"
	"personal-search/internal/embed"
	"personal-search/internal/ingest"
	"personal-search/internal/qdrant"
)

func main() {
	var exportPath = flag.String("export", "", "Path to ChatGPT export JSON or ZIP")
	var qdrantURL = flag.String("qdrant", "http://localhost:6333", "Qdrant base URL")
	var collection = flag.String("collection", "personal_docs", "Qdrant collection name")
	var embedder = flag.String("embedder", "openai", "Embedding provider: openai|fake|ollama")
	var openaiKey = flag.String("openai_key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
	var modelName = flag.String("model", embed.DefaultOpenAIModel, "OpenAI embedding model")
	var ollamaURL = flag.String("ollama_url", embed.DefaultOllamaURL, "Ollama base URL")
	var ollamaModel = flag.String("ollama_model", embed.DefaultOllamaModel, "Ollama embedding model")
	var dim = flag.Int("dim", 0, "Embedding dimension (0 = infer for Ollama)")
	var batchSize = flag.Int("batch", 64, "Embedding batch size")
	var maxDocs = flag.Int("max_docs", 0, "Optional cap on number of documents (0 = all)")
	var reportPath = flag.String("report-out", ingest.DefaultReportPath(), "Path for the ingest report")
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
	emb, err := embed.New(*embedder, *openaiKey, *modelName, *ollamaURL, *ollamaModel, *dim)
	if err != nil {
		fatal(err.Error())
	}
	result, err := ingest.Run(context.Background(), ingest.Options{InputPath: *exportPath, ReportPath: *reportPath, Collection: *collection, BatchSize: *batchSize, MaxDocs: *maxDocs}, chatgpt.SourceAdapter{}, emb, qdrant.NewDocumentStore(qdrant.New(*qdrantURL)))
	if result.Report != nil {
		summary := result.Report.Counts()
		fmt.Printf("classification summary: total=%d documents=%d skipped=%d failed=%d\n", summary.TotalRecords, summary.DocumentsCreated, summary.Skipped, summary.Failed)
	}
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("ingestion complete: %d documents into %s\n", result.Documents, *collection)
	fmt.Printf("ingest report: %s\n", *reportPath)
}

func fatal(msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(1) }
