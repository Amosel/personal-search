package chatgpt

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestToDocumentsWithReport_PreservesBehaviorAndTracksOutcomes(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID:    "conv1",
				Title: "T",
				Mapping: map[string]Message{
					"nil": {ID: "nil", Message: nil},
					"sys": {
						ID: "sys",
						Message: &MsgBody{
							Author:     Author{Role: "system"},
							CreateTime: 1,
							Content:    Content{Parts: []any{"x"}},
						},
					},
					"empty": {
						ID: "empty",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 2,
							Content:    Content{Parts: []any{"   "}},
						},
					},
					"badts": {
						ID: "badts",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 0,
							Content:    Content{Parts: []any{"x"}},
						},
					},
					"ok": {
						ID: "ok",
						Message: &MsgBody{
							Author:     Author{Role: "assistant"},
							CreateTime: 3,
							Content:    Content{Parts: []any{"hello"}},
						},
					},
				},
			},
		},
	}

	report := NewIngestReport("export.json")
	docs, err := ToDocumentsWithReport(exp, report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	if report.Summary.TotalRecords != 5 {
		t.Fatalf("expected 5 records, got %d", report.Summary.TotalRecords)
	}
	if report.Summary.DocumentsCreated != 1 || report.Summary.Skipped != 4 || report.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if report.Summary.ByReason[string(ReasonDocumentCreated)] != 1 {
		t.Fatalf("unexpected reason counts: %+v", report.Summary.ByReason)
	}
	if report.Progress.Classified != 1 || report.Progress.Skipped != 4 || report.Progress.Selected != 0 {
		t.Fatalf("classification changed ingestion selection: %+v", report.Progress)
	}
}

func TestIngestReportWriteReplacesPrivatelyAndAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	oldData := []byte(`{"status":"old"}`)
	if err := os.WriteFile(path, oldData, 0644); err != nil {
		t.Fatal(err)
	}
	oldFile, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer oldFile.Close()
	report := NewIngestReport("synthetic.json")
	report.MarkCompleted()
	if err := report.Write(path); err != nil {
		t.Fatal(err)
	}
	// An already-open reader retains the old complete file after replacement.
	retained, err := io.ReadAll(oldFile)
	if err != nil || string(retained) != string(oldData) {
		t.Fatalf("old file was modified in place: data=%q err=%v", retained, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved IngestReport
	if err := json.Unmarshal(data, &saved); err != nil || saved.Status != "completed" || saved.SchemaVersion != 1 {
		t.Fatalf("invalid published report: data=%s err=%v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("report permissions are not private: info=%v err=%v", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary report leaked: entries=%v err=%v", entries, err)
	}
}

func TestLegacyIngestReportRemainsReadable(t *testing.T) {
	var report IngestReport
	data := []byte(`{"source":"chatgpt","status":"completed","summary":{"total_records":3,"documents_created":2,"skipped":1,"failed":0,"by_reason":{"document_created":2}},"entries":[]}`)
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 0 || report.Summary.DocumentsCreated != 2 || report.Summary.Skipped != 1 || report.Progress.Acknowledged != 0 {
		t.Fatalf("legacy report was reinterpreted: %+v", report)
	}
}

func TestIngestReport_Write(t *testing.T) {
	report := NewIngestReport("export.json")
	report.Append(ClassificationEntry{
		RecordID: SourceRecordID{Source: "chatgpt", ConversationID: "c", MessageID: "m"},
		Outcome:  OutcomeSkip,
		Reason:   ReasonSkipNilMessage,
	})
	report.MarkCompleted()
	path := filepath.Join(t.TempDir(), "ingest_report.json")
	if err := report.Write(path); err != nil {
		t.Fatalf("write report: %v", err)
	}
}
