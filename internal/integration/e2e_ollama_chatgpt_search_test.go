package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"personal-search/internal/model"
	"personal-search/internal/qdrant"
)

func TestE2E_OllamaIngestCLIThenSearchAndMCP(t *testing.T) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "nomic-embed-text:latest"
	}

	qc := qdrant.New(qdrantURL)
	if err := qc.Healthy(context.Background()); err != nil {
		t.Skipf("qdrant not reachable at %s: %v", qdrantURL, err)
	}
	if !ollamaModelAvailable(t, ollamaURL, ollamaModel) {
		t.Skipf("ollama model %s not available at %s", ollamaModel, ollamaURL)
	}

	root := repoRootForServer(t)
	exportPath := filepath.Join(root, "internal", "integration", "testdata", "chatgpt_export_valid.json")
	collection := fmt.Sprintf("itest_ollama_chatgpt_%d", time.Now().UnixNano())

	cmd := exec.Command(
		"go", "run", "./cmd/ingest_chatgpt",
		"--export", exportPath,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "ollama",
		"--ollama_url", ollamaURL,
		"--ollama_model", ollamaModel,
		"--dim", "0",
		"--batch", "2",
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ollama ingest failed: %v\noutput:\n%s", err, string(out))
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
		"--embedder", "ollama",
		"--ollama_url", ollamaURL,
		"--ollama_model", ollamaModel,
		"--dim", "0",
	)
	server.Dir = root
	serverOut := new(bytes.Buffer)
	server.Stdout = serverOut
	server.Stderr = serverOut
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	}()
	if err := waitForHealth("http://" + addr + "/health"); err != nil {
		t.Fatalf("server health check failed: %v\nserver output:\n%s", err, serverOut.String())
	}

	t.Run("HTTPSearch", func(t *testing.T) {
		reqBody := model.SearchRequest{Query: "custody strategy planning notes", Limit: 3}
		reqJSON, _ := json.Marshal(reqBody)
		resp, err := http.Post("http://"+addr+"/search", "application/json", bytes.NewReader(reqJSON))
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var out model.SearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(out.Results) == 0 {
			t.Fatal("expected results")
		}
	})

	t.Run("SearchCLI", func(t *testing.T) {
		cmd := exec.Command(
			"go", "run", "./cmd/search_chatgpt",
			"--server", "http://"+addr,
			"--query", "shopping list budget",
			"--thread_id", "conv-beta",
		)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("search cli failed: %v", err)
		}
		if !bytes.Contains(out, []byte("conv-beta")) {
			t.Fatalf("expected conv-beta in cli output, got %s", string(out))
		}
	})

	t.Run("MCP", func(t *testing.T) {
		cmd := exec.Command(
			"go", "run", "./cmd/mcp_chatgpt_search",
			"--server", "http://"+addr,
		)
		cmd.Dir = root
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatalf("stdout pipe: %v", err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatalf("start mcp server: %v", err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}()
		reader := bufio.NewReader(stdout)
		writeFrame(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
		_ = readFrame(t, reader)
		writeFrame(t, stdin, map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "search_documents",
				"arguments": map[string]any{
					"query":  "shopping list",
					"author": "assistant",
				},
			},
		})
		resp := readFrame(t, reader)
		raw, _ := json.Marshal(resp)
		if !bytes.Contains(raw, []byte("chatgpt")) {
			t.Fatalf("expected chatgpt results, got %s", string(raw))
		}
	})
}

func ollamaModelAvailable(t *testing.T, ollamaURL, model string) bool {
	t.Helper()
	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false
	}
	for _, m := range out.Models {
		if m.Name == model {
			return true
		}
	}
	return false
}
