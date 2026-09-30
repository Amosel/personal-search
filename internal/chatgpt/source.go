package chatgpt

import (
	"personal-search/internal/ingestreport"
	"personal-search/internal/model"
)

// SourceAdapter converts a ChatGPT export into canonical Documents while
// recording per-message classification in the existing ingest report.
type SourceAdapter struct{}

func (SourceAdapter) LoadDocuments(path string) ([]model.Document, ingestreport.Report, error) {
	report := NewIngestReport(path)
	exp, err := LoadExport(path)
	if err != nil {
		return nil, report, err
	}
	docs, err := ToDocumentsWithReport(exp, report)
	return docs, report, err
}
