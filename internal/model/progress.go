package model

import "fmt"

// BatchRange indexes the selected documents, with an exclusive End.
type BatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// IngestProgress counts storage outcomes. NotAttempted includes documents whose
// embedding failed; Uncertain means a write was sent without acknowledgement.
type IngestProgress struct {
	Classified       int          `json:"classified"`
	Selected         int          `json:"selected"`
	Acknowledged     int          `json:"acknowledged"`
	NotAttempted     int          `json:"not_attempted"`
	Skipped          int          `json:"skipped"`
	Excluded         int          `json:"excluded_by_limit"`
	Uncertain        int          `json:"uncertain"`
	UncertainBatches []BatchRange `json:"uncertain_batches,omitempty"`
}

func (p *IngestProgress) Recount() { p.NotAttempted = p.Selected - p.Acknowledged - p.Uncertain }

// IngestError preserves both the cause and the work already done.
type IngestError struct {
	Stage    string
	Batch    *BatchRange
	Progress IngestProgress
	Err      error
}

func (e *IngestError) Error() string {
	if e.Batch != nil {
		return fmt.Sprintf("%s batch [%d:%d]: %v (acknowledged=%d uncertain=%d not_attempted=%d)", e.Stage, e.Batch.Start, e.Batch.End, e.Err, e.Progress.Acknowledged, e.Progress.Uncertain, e.Progress.NotAttempted)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}
func (e *IngestError) Unwrap() error { return e.Err }
