package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

func TestE2E_IngestThenHTTPServerSearch(t *testing.T) {
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
	collection := fmt.Sprintf("itest_e2e_server_%d", time.Now().UnixNano())

	ingest := exec.Command(
		"go", "run", "./cmd/ingest_chatgpt",
		"--export", exportPath,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "fake",
		"--dim", "16",
		"--batch", "2",
		"--report-out", filepath.Join(t.TempDir(), "ingest_report.json"),
	)
	ingest.Dir = root
	ingestOut, err := ingest.CombinedOutput()
	if err != nil {
		t.Fatalf("ingest failed: %v\noutput:\n%s", err, string(ingestOut))
	}
	t.Cleanup(func() {
		_ = qc.DeleteCollection(context.Background(), collection)
	})

	addr := "127.0.0.1:" + freePort(t)
	server := exec.Command(
		"go", "run", "./cmd/server",
		"--addr", addr,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "fake",
		"--dim", "16",
	)
	server.Dir = root
	serverOut := new(bytes.Buffer)
	server.Stdout = serverOut
	server.Stderr = serverOut
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	})

	if err := waitForHealth("http://" + addr + "/health"); err != nil {
		t.Fatalf("server health check failed: %v\nserver output:\n%s", err, serverOut.String())
	}

	reqBody := model.SearchRequest{
		Query: "custody strategy planning notes",
		Limit: 5,
	}
	reqJSON, _ := json.Marshal(reqBody)

	resp, err := http.Post("http://"+addr+"/search", "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		t.Fatalf("search request failed: %v\nserver output:\n%s", err, serverOut.String())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nserver output:\n%s", resp.StatusCode, serverOut.String())
	}

	var out model.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Results) == 0 {
		t.Fatalf("expected results, got none\nserver output:\n%s", serverOut.String())
	}
	if out.Results[0].Source != "chatgpt" {
		t.Fatalf("expected source chatgpt, got %q", out.Results[0].Source)
	}
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", url)
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc free port: %v", err)
	}
	defer l.Close()
	_, p, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	return p
}

func repoRootForServer(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
