package model

import "testing"

func TestFiltersValidate_RejectsKeywords(t *testing.T) {
	f := &Filters{Keywords: []string{"custody"}}

	err := f.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported keywords filter")
	}
}
