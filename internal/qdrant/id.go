package qdrant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// PointID preserves Qdrant's uint64-or-UUID wire type without a float64 conversion.
// Its zero value is the valid numeric ID zero. Optional cursors use *PointID.
type PointID struct {
	number uint64
	uuid   string
}

func NumericID(n uint64) PointID { return PointID{number: n} }

func UUIDID(s string) (PointID, error) {
	if len(s) != 36 {
		return PointID{}, fmt.Errorf("invalid point UUID %q", s)
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return PointID{}, fmt.Errorf("invalid point UUID %q", s)
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return PointID{}, fmt.Errorf("invalid canonical point UUID %q", s)
		}
	}
	return PointID{uuid: s}, nil
}

func (id PointID) String() string {
	if id.uuid != "" {
		return id.uuid
	}
	return strconv.FormatUint(id.number, 10)
}

func (id PointID) MarshalJSON() ([]byte, error) {
	if id.uuid != "" {
		return json.Marshal(id.uuid)
	}
	return []byte(strconv.FormatUint(id.number, 10)), nil
}

func (id *PointID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		v, err := UUIDID(s)
		if err == nil {
			*id = v
		}
		return err
	}
	if len(b) == 0 {
		return fmt.Errorf("empty point ID")
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return fmt.Errorf("point ID must be a uint64 or canonical UUID")
		}
	}
	if len(b) > 1 && b[0] == '0' {
		return fmt.Errorf("point ID has a leading zero")
	}
	n, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}
	*id = NumericID(n)
	return nil
}

// Pagination rejects malformed or non-progressing continuations in scroll loops.
type Pagination struct {
	Offset *PointID
	seen   map[PointID]bool
}

func (p *Pagination) Advance(page *ScrollResponse) (bool, error) {
	next := page.Result.NextPageOffset
	if next == nil {
		return false, nil
	}
	if len(page.Result.Points) == 0 {
		return false, fmt.Errorf("empty scroll page with continuation")
	}
	if p.seen == nil {
		p.seen = make(map[PointID]bool)
	}
	if p.seen[*next] {
		return false, fmt.Errorf("repeated scroll cursor %s", next.String())
	}
	p.seen[*next] = true
	p.Offset = next
	return true, nil
}
