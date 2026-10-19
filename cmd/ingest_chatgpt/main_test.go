package main

import (
	"bytes"
	"personal-search/internal/chatgpt"
	"personal-search/internal/ingest"
	"personal-search/internal/model"
	"strings"
	"testing"
)

func TestSummaryDistinguishesClassifiedAndStorageOutcomes(t *testing.T) {
	var out bytes.Buffer
	writeSummary(&out, ingest.Result{Report: chatgpt.NewIngestReport("synthetic"), Progress: model.IngestProgress{Classified: 8, Selected: 6, Acknowledged: 2, Uncertain: 2, NotAttempted: 2, Skipped: 1, Excluded: 2}})
	if !strings.Contains(out.String(), "classified=8 selected=6 acknowledged=2 uncertain=2 not_attempted=2 skipped=1 excluded_by_limit=2") {
		t.Fatal(out.String())
	}
}
