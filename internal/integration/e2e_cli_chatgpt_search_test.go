package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

func TestE2E_ChatGPTSearchCLI(t *testing.T) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}

	qc := qdrant.New(qdrantURL)
	if err := qc.Healthy(context.Background()); err != nil {
		t.Skipf("qdrant not reachable at %s: %v", qdrantURL, err)
	}

	root := repoRootForServer(t)
	exportPath := filepath.Join(root, "internal", "integration", "testdata", "chatgpt_export_valid.json")
	collection := fmt.Sprintf("itest_cli_chatgpt_%d", time.Now().UnixNano())

	runIngestCLI(t, root, qdrantURL, collection, exportPath)
	t.Cleanup(func() {
		_ = qc.DeleteCollection(context.Background(), collection)
	})

	addr, stop := startServerCLI(t, root, qdrantURL, collection)
	defer stop()

	cmd := exec.Command(
		"go", "run", "./cmd/search_chatgpt",
		"--server", "http://"+addr,
		"--query", "custody strategy",
		"--author", "user",
	)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("search cli failed: %v", err)
	}

	var resp model.SearchResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode cli response: %v\n%s", err, string(out))
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected search results")
	}
	for _, r := range resp.Results {
		if r.Source != "chatgpt" {
			t.Fatalf("unexpected source: %+v", r)
		}
		if r.Metadata["author"] != "user" {
			t.Fatalf("unexpected author metadata: %+v", r.Metadata)
		}
	}
}
