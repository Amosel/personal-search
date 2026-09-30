package integration

import (
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

func TestAcceptance_E2E(t *testing.T) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}

	ctx := context.Background()
	qc := qdrant.New(qdrantURL)
	if err := qc.Healthy(ctx); err != nil {
		t.Skipf("qdrant not reachable at %s: %v", qdrantURL, err)
	}

	root := repoRootForServer(t)
	exportPath := filepath.Join(root, "internal", "integration", "testdata", "chatgpt_export_valid.json")
	collection := fmt.Sprintf("itest_acceptance_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = qc.DeleteCollection(context.Background(), collection) })

	reportPath := filepath.Join(t.TempDir(), "ingest_report.json")
	runIngestCLIWithReport(t, root, qdrantURL, collection, exportPath, reportPath)
	assertCompletedIngestReport(t, reportPath)

	initialCount, err := qc.Count(ctx, collection)
	if err != nil {
		t.Fatalf("count after first ingest: %v", err)
	}
	if initialCount == 0 {
		t.Fatal("expected points after ingest")
	}

	runIngestCLIWithReport(t, root, qdrantURL, collection, exportPath, reportPath)
	secondCount, err := qc.Count(ctx, collection)
	if err != nil {
		t.Fatalf("count after second ingest: %v", err)
	}
	if secondCount != initialCount {
		t.Fatalf("re-ingest changed point count: first=%d second=%d", initialCount, secondCount)
	}

	addr, stop := startServerCLI(t, root, qdrantURL, collection)
	defer stop()

	t.Run("Health", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			t.Fatalf("health request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("HappyPathAndContract", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "custody strategy planning notes",
			Limit: 5,
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if len(out.Results) == 0 {
			t.Fatal("expected search results")
		}
		r := out.Results[0]
		if r.ID == "" || r.Text == "" || r.Source == "" || r.Timestamp == "" {
			t.Fatalf("missing required result fields: %+v", r)
		}
		if r.Metadata["author"] == nil || r.Metadata["thread_id"] == nil {
			t.Fatalf("missing required metadata fields: %+v", r.Metadata)
		}
	})

	t.Run("Validation_EmptyQuery", func(t *testing.T) {
		status, _ := doSearchJSON(t, addr, model.SearchRequest{Query: ""})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	t.Run("Validation_InvalidJSON", func(t *testing.T) {
		resp, err := http.Post("http://"+addr+"/search", "application/json", bytes.NewBufferString("{"))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Validation_InvalidDate", func(t *testing.T) {
		status, _ := doSearchJSON(t, addr, model.SearchRequest{
			Query: "notes",
			Filters: &model.Filters{
				Date: &model.DateRange{From: "2024-13-01"},
			},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	t.Run("Validation_KeywordsUnsupported", func(t *testing.T) {
		status, _ := doSearchJSON(t, addr, model.SearchRequest{
			Query: "notes",
			Filters: &model.Filters{
				Keywords: []string{"custody"},
			},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	t.Run("Determinism_Top1Stable", func(t *testing.T) {
		status1, out1 := doSearchJSON(t, addr, model.SearchRequest{Query: "custody strategy", Limit: 5})
		status2, out2 := doSearchJSON(t, addr, model.SearchRequest{Query: "custody strategy", Limit: 5})
		if status1 != http.StatusOK || status2 != http.StatusOK {
			t.Fatalf("expected 200/200, got %d/%d", status1, status2)
		}
		if len(out1.Results) == 0 || len(out2.Results) == 0 {
			t.Fatal("expected results in both calls")
		}
		if out1.Results[0].ID != out2.Results[0].ID {
			t.Fatalf("top result unstable: %s vs %s", out1.Results[0].ID, out2.Results[0].ID)
		}
	})

	t.Run("Filter_Author", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "shopping list budget",
			Limit: 10,
			Filters: &model.Filters{
				Author: "assistant",
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		for _, r := range out.Results {
			if r.Metadata["author"] != "assistant" {
				t.Fatalf("unexpected author in result: %+v", r.Metadata)
			}
		}
	})

	t.Run("Filter_Thread", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "shopping list budget",
			Limit: 10,
			Filters: &model.Filters{
				ThreadID: "conv-beta",
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		for _, r := range out.Results {
			if r.Metadata["thread_id"] != "conv-beta" {
				t.Fatalf("unexpected thread_id in result: %+v", r.Metadata)
			}
		}
	})

	t.Run("Filter_DateRange", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "notes planning",
			Limit: 10,
			Filters: &model.Filters{
				Date: &model.DateRange{
					From: "2024-12-31",
					To:   "2024-12-31",
				},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		for _, r := range out.Results {
			if r.Metadata["thread_id"] != "conv-alpha" {
				t.Fatalf("date filter leaked unexpected thread: %+v", r.Metadata)
			}
		}
	})

	t.Run("Filter_SourceNoMatch", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "notes",
			Limit: 5,
			Filters: &model.Filters{
				Source: []string{"email"},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if len(out.Results) != 0 {
			t.Fatalf("expected empty results, got %d", len(out.Results))
		}
	})

	t.Run("Filter_SourceOr", func(t *testing.T) {
		status, out := doSearchJSON(t, addr, model.SearchRequest{
			Query: "shopping list budget",
			Limit: 5,
			Filters: &model.Filters{
				Source: []string{"email", "chatgpt"},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if len(out.Results) == 0 {
			t.Fatal("expected non-empty results with source OR filter")
		}
		for _, r := range out.Results {
			if r.Source != "chatgpt" {
				t.Fatalf("unexpected source in result: %+v", r)
			}
		}
	})

	t.Run("EmptyCollectionReturnsEmptyResults", func(t *testing.T) {
		emptyCollection := fmt.Sprintf("itest_acceptance_empty_%d", time.Now().UnixNano())
		if err := qc.EnsureCollection(ctx, emptyCollection, 16); err != nil {
			t.Fatalf("ensure empty collection: %v", err)
		}
		defer qc.DeleteCollection(context.Background(), emptyCollection)

		emptyAddr, stopEmpty := startServerCLI(t, root, qdrantURL, emptyCollection)
		defer stopEmpty()

		status, out := doSearchJSON(t, emptyAddr, model.SearchRequest{
			Query: "anything",
			Limit: 5,
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if len(out.Results) != 0 {
			t.Fatalf("expected empty results, got %d", len(out.Results))
		}
	})
}

func runIngestCLI(t *testing.T, root, qdrantURL, collection, exportPath string) {
	t.Helper()
	runIngestCLIWithReport(t, root, qdrantURL, collection, exportPath, filepath.Join(t.TempDir(), "ingest_report.json"))
}

func runIngestCLIWithReport(t *testing.T, root, qdrantURL, collection, exportPath, reportPath string) {
	t.Helper()
	cmd := exec.Command(
		"go", "run", "./cmd/ingest_chatgpt",
		"--export", exportPath,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "fake",
		"--dim", "16",
		"--batch", "2",
		"--report-out", reportPath,
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ingest failed: %v\noutput:\n%s", err, string(out))
	}
}

func assertCompletedIngestReport(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ingest report: %v", err)
	}
	var report struct {
		Status  string `json:"status"`
		Summary struct {
			TotalRecords     int `json:"total_records"`
			DocumentsCreated int `json:"documents_created"`
			Skipped          int `json:"skipped"`
			Failed           int `json:"failed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode ingest report: %v", err)
	}
	if report.Status != "completed" {
		t.Fatalf("expected completed report, got %q", report.Status)
	}
	if report.Summary.TotalRecords == 0 || report.Summary.DocumentsCreated == 0 || report.Summary.Failed != 0 {
		t.Fatalf("unexpected report summary: %+v", report.Summary)
	}
}

func startServerCLI(t *testing.T, root, qdrantURL, collection string) (string, func()) {
	t.Helper()
	addr := "127.0.0.1:" + freePort(t)
	cmd := exec.Command(
		"go", "run", "./cmd/server",
		"--addr", addr,
		"--qdrant", qdrantURL,
		"--collection", collection,
		"--embedder", "fake",
		"--dim", "16",
	)
	cmd.Dir = root
	out := new(bytes.Buffer)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	if err := waitForHealth("http://" + addr + "/health"); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("wait for health: %v\nserver output:\n%s", err, out.String())
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return addr, stop
}

func doSearchJSON(t *testing.T, addr string, req model.SearchRequest) (int, model.SearchResponse) {
	t.Helper()
	var out model.SearchResponse
	b, _ := json.Marshal(req)
	resp, err := http.Post("http://"+addr+"/search", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("search request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode search response: %v", err)
		}
	}
	return resp.StatusCode, out
}
