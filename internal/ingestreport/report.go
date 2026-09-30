// Package ingestreport defines the source-neutral lifecycle contract for
// ingestion reports. Source adapters retain ownership of report contents.
package ingestreport

type Summary struct {
	TotalRecords     int `json:"total_records"`
	DocumentsCreated int `json:"documents_created"`
	Skipped          int `json:"skipped"`
	Failed           int `json:"failed"`
}

// Report exposes only lifecycle operations shared by the ingestion pipeline.
// Counts and serialized fields remain owned by the source adapter.
type Report interface {
	MarkFailed(stage, message string)
	MarkCompleted()
	Write(path string) error
	Counts() Summary
	ReportState() string
}
