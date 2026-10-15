package chatgpt

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	Stage   string      `json:"stage"`
	Message string      `json:"message"`
	Batch   *BatchRange `json:"batch,omitempty"`
}

// BatchRange uses zero-based indexes into the selected documents, with an
// exclusive end. An uncertain batch may have been applied by the store.
type BatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// IngestProgress separates source classification from work sent to the store.
// Classified counts canonical documents; Skipped counts source records.
// Selected = Acknowledged + Uncertain + NotAttempted on every return.
type IngestProgress struct {
	Classified        int         `json:"classified"`
	Selected          int         `json:"selected"`
	Acknowledged      int         `json:"acknowledged"`
	Uncertain         int         `json:"uncertain"`
	NotAttempted      int         `json:"not_attempted"`
	Skipped           int         `json:"skipped"`
	ExcludedByMaxDocs int         `json:"excluded_by_max_docs"`
	UncertainBatch    *BatchRange `json:"uncertain_batch,omitempty"`
}

type IngestReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Source        string                `json:"source"`
	ExportPath    string                `json:"export_path"`
	StartedAt     string                `json:"started_at"`
	FinishedAt    string                `json:"finished_at,omitempty"`
	Status        string                `json:"status"`
	Summary       IngestReportSummary   `json:"summary"`
	Entries       []ClassificationEntry `json:"entries"`
	Failure       *IngestReportFailure  `json:"failure,omitempty"`
	Progress      IngestProgress        `json:"progress"`
}

func NewIngestReport(exportPath string) *IngestReport {
	return &IngestReport{
		SchemaVersion: 1,
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
		r.Progress.Classified++
	case OutcomeSkip:
		r.Summary.Skipped++
		r.Progress.Skipped++
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
	return writeReportAtomic(path, data)
}

// writeReportAtomic keeps both temporary and published reports private. A
// failed write leaves the previous report intact; it cannot describe this run.
func writeReportAtomic(path string, data []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".ingest-report-*")
	if err != nil {
		return err
	}
	name := f.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			if cleanupErr := os.Remove(name); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
