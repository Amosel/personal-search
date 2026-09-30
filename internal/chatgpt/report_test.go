package chatgpt

import (
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
