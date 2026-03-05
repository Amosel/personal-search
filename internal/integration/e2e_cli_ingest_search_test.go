package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"personal-search/internal/embed"
	"personal-search/internal/qdrant"
)

func TestE2E_CLIIngestThenSearch(t *testing.T) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}

	ctx := context.Background()
	qc := qdrant.New(qdrantURL)
	if err := qc.Healthy(ctx); err != nil {
		t.Skipf("qdrant not reachable at %s: %v", qdrantURL, err)
	}

	root := repoRoot(t)
	exportPath := filepath.Join(root, "internal", "integration", "testdata", "chatgpt_export_valid.json")
	collection := fmt.Sprintf("itest_e2e_cli_%d", time.Now().UnixNano())

	cmd := exec.Command(
		"go", "run", "./cmd/ingest_chatgpt",
		"--export", exportPath,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "fake",
		"--dim", "16",
		"--batch", "2",
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ingest CLI failed: %v\noutput:\n%s", err, string(out))
	}
	t.Cleanup(func() {
		_ = qc.DeleteCollection(context.Background(), collection)
	})

	emb := &embed.FakeEmbedder{DimVal: 16}
	qvecs, err := emb.Embed(ctx, []string{"custody strategy planning notes"})
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}

	res, err := qc.Search(ctx, collection, qdrant.SearchRequest{
		Vector:      qvecs[0],
		Limit:       3,
		WithPayload: true,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(res.Result) == 0 {
		t.Fatalf("expected results after CLI ingest, got none; cli output:\n%s", string(out))
	}

	top := res.Result[0]
	if top.Payload["doc_id"] == nil {
		t.Fatalf("expected doc_id in payload; payload=%v", top.Payload)
	}
	if top.Payload["source"] != "chatgpt" {
		t.Fatalf("expected source=chatgpt; payload=%v", top.Payload)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
