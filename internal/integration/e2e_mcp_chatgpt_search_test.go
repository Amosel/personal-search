package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"personal-search/internal/qdrant"
)

func TestE2E_MCPChatGPTSearch(t *testing.T) {
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
	collection := fmt.Sprintf("itest_mcp_chatgpt_%d", time.Now().UnixNano())

	runIngestCLI(t, root, qdrantURL, collection, exportPath)
	t.Cleanup(func() {
		_ = qc.DeleteCollection(context.Background(), collection)
	})

	addr, stop := startServerCLI(t, root, qdrantURL, collection)
	defer stop()

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
	writeFrame(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	_ = readFrame(t, reader)
	writeFrame(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search_documents",
			"arguments": map[string]any{
				"query":     "shopping list",
				"thread_id": "conv-beta",
			},
		},
	})
	resp := readFrame(t, reader)
	raw, _ := json.Marshal(resp)
	if !bytes.Contains(raw, []byte("conv-beta")) {
		t.Fatalf("expected thread-scoped result, got %s", string(raw))
	}

	writeFrame(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search_documents",
			"arguments": map[string]any{
				"query":    "shopping list",
				"keywords": []string{"budget"},
			},
		},
	})
	errResp := readFrame(t, reader)
	if errResp["error"] == nil {
		t.Fatalf("expected MCP error response, got %v", errResp)
	}
}

func writeFrame(t *testing.T, w io.Writer, msg map[string]any) {
	t.Helper()
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func readFrame(t *testing.T, br *bufio.Reader) map[string]any {
	t.Helper()
	header, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if !strings.HasPrefix(strings.ToLower(header), "content-length:") {
		t.Fatalf("unexpected header: %q", header)
	}
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatalf("read header terminator: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:")))
	if err != nil {
		t.Fatalf("parse content length: %v", err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(br, body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode frame: %v\n%s", err, string(body))
	}
	return out
}
