package main

import (
	"bytes"
	"strings"
	"testing"

	"personal-search/internal/chatgpt"
	"personal-search/internal/ingest"
)

func TestSummaryDistinguishesClassificationAndPartialProgress(t *testing.T) {
	report := chatgpt.NewIngestReport("synthetic.json")
	report.Summary = chatgpt.IngestReportSummary{TotalRecords: 7, DocumentsCreated: 6, Skipped: 1}
	report.MarkFailed("upsert", "synthetic uncertain completion")
	result := ingest.Result{Documents: 2, Report: report, Progress: chatgpt.IngestProgress{
		Classified: 6, Selected: 5, Acknowledged: 2, Uncertain: 2, NotAttempted: 1, Skipped: 1, ExcludedByMaxDocs: 1,
	}}
	var output bytes.Buffer
	writeSummary(&output, result)
	for _, want := range []string{
		"classification summary: total=7 documents=6 skipped=1 failed=0",
		"classified=6 selected=5 acknowledged=2 uncertain=2 not_attempted=1 skipped=1 excluded_by_max_docs=1",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("summary = %q, missing %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "complete") {
		t.Fatalf("partial failure printed completion: %q", output.String())
	}
}
