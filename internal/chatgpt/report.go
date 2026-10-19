package chatgpt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"personal-search/internal/model"
	"time"
)

type ClassificationOutcome string

const (
	OutcomeDocument ClassificationOutcome = "document"
	OutcomeSkip     ClassificationOutcome = "skip"
	OutcomeFail     ClassificationOutcome = "fail"
)

type ClassificationReason string

const (
	ReasonDocumentCreated         ClassificationReason = "document_created"
	ReasonSkipUnsupportedContent  ClassificationReason = "skip_unsupported_content"
	ReasonSkipNilMessage          ClassificationReason = "skip_nil_message"
	ReasonSkipSystemMessage       ClassificationReason = "skip_system_message"
	ReasonSkipEmptyTextAfterCanon ClassificationReason = "skip_empty_text_after_canonicalize"
	ReasonSkipInvalidTimestamp    ClassificationReason = "skip_invalid_timestamp"
	ReasonFailDocumentValidation  ClassificationReason = "fail_document_validation"
)

type SourceRecordID struct {
	Source         string `json:"source"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

type ClassificationEntry struct {
	RecordID        SourceRecordID        `json:"record_id"`
	Outcome         ClassificationOutcome `json:"outcome"`
	Reason          ClassificationReason  `json:"reason"`
	Author          string                `json:"author,omitempty"`
	TimestampUnixMS int64                 `json:"timestamp_unix_ms,omitempty"`
	DocumentID      string                `json:"document_id,omitempty"`
	TextLen         int                   `json:"text_len,omitempty"`
	Error           string                `json:"error,omitempty"`
}

type IngestReportSummary struct {
	TotalRecords     int            `json:"total_records"`
	DocumentsCreated int            `json:"documents_created"`
	Skipped          int            `json:"skipped"`
	Failed           int            `json:"failed"`
	ByReason         map[string]int `json:"by_reason"`
}

type IngestReportFailure struct {
	Stage   string            `json:"stage"`
	Message string            `json:"message"`
	Batch   *model.BatchRange `json:"batch,omitempty"`
}

type IngestReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Progress      model.IngestProgress  `json:"progress"`
	Source        string                `json:"source"`
	ExportPath    string                `json:"export_path"`
	StartedAt     string                `json:"started_at"`
	FinishedAt    string                `json:"finished_at,omitempty"`
	Status        string                `json:"status"`
	Summary       IngestReportSummary   `json:"summary"`
	Entries       []ClassificationEntry `json:"entries"`
	Failure       *IngestReportFailure  `json:"failure,omitempty"`
}

func NewIngestReport(exportPath string) *IngestReport {
	return &IngestReport{
		SchemaVersion: 2,
		Source:        "chatgpt",
		ExportPath:    exportPath,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:        "running",
		Summary: IngestReportSummary{
			ByReason: make(map[string]int),
		},
		Entries: make([]ClassificationEntry, 0),
	}
}

func (r *IngestReport) Append(entry ClassificationEntry) {
	if r == nil {
		return
	}
	r.Entries = append(r.Entries, entry)
	r.Summary.TotalRecords++
	r.Summary.ByReason[string(entry.Reason)]++
	switch entry.Outcome {
	case OutcomeDocument:
		r.Summary.DocumentsCreated++
	case OutcomeSkip:
		r.Summary.Skipped++
	case OutcomeFail:
		r.Summary.Failed++
	}
}

func (r *IngestReport) MarkFailed(stage, message string) {
	if r == nil {
		return
	}
	r.Status = "failed"
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.Failure = &IngestReportFailure{Stage: stage, Message: message}
}

func (r *IngestReport) MarkCompleted() {
	if r == nil {
		return
	}
	r.Status = "completed"
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339)
}

func (r *IngestReport) Write(path string) error {
	if r == nil {
		return nil
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ingest-report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
