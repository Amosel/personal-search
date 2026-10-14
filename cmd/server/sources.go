package main

import (
	"fmt"
	"personal-search/internal/model"
)

func searchSources(filters *model.Filters) ([]string, error) {
	sources := []string{"chatgpt"}
	if filters != nil && len(filters.Source) > 0 {
		sources = filters.Source
	}
	if len(sources) == 1 && sources[0] == "" {
		sources = []string{"chatgpt"}
	}
	if len(sources) == 1 && sources[0] == "all" {
		sources = []string{"chatgpt", "google_search", "apple_notes", "voicememo"}
	}
	seen := map[string]bool{}
	for _, source := range sources {
		switch source {
		case "", "all":
			return nil, fmt.Errorf("invalid source selection: %s", source)
		}
		if seen[source] {
			return nil, fmt.Errorf("duplicate source: %s", source)
		}
		seen[source] = true
	}
	if filters != nil && (filters.Author != "" || filters.ThreadID != "") && (len(sources) != 1 || sources[0] != "chatgpt") {
		return nil, fmt.Errorf("speaker and conversation filters apply only to ChatGPT")
	}
	return sources, nil
}
