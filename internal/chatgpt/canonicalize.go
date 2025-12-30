package chatgpt

import "strings"

// CanonicalizeText normalizes raw text by removing excessive whitespace
// and collapsing blank lines. Returns empty string for whitespace-only input.
func CanonicalizeText(raw string) string {
	// Normalize line endings
	s := strings.ReplaceAll(raw, "\r\n", "\n")

	// Split into lines and process
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blankCount := 0

	for _, line := range lines {
		// Trim whitespace from each line
		line = strings.TrimSpace(line)

		// Check if line is blank
		if line == "" {
			blankCount++
			// Keep at most one consecutive blank line
			if blankCount <= 1 {
				out = append(out, "")
			}
			continue
		}

		blankCount = 0
		out = append(out, line)
	}

	// Join and trim the result
	result := strings.TrimSpace(strings.Join(out, "\n"))
	return result
}
