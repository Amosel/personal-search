package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"personal-search/internal/chatgptsearch"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, _ io.Writer, searcher chatgptsearch.Searcher) error {
	fs := flag.NewFlagSet("search_chatgpt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	query := fs.String("query", "", "Search query")
	server := fs.String("server", "http://127.0.0.1:8080", "Search server base URL")
	limit := fs.Int("limit", 10, "Max results")
	format := fs.String("format", "json", "Output format: json|yaml")
	author := fs.String("author", "", "Filter by author")
	threadID := fs.String("thread_id", "", "Filter by thread ID")
	from := fs.String("from", "", "Filter from date YYYY-MM-DD")
	to := fs.String("to", "", "Filter to date YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *query == "" {
		return fmt.Errorf("--query is required")
	}

	if searcher == nil {
		searcher = &chatgptsearch.HTTPClient{BaseURL: *server}
	}

	resp, err := searcher.Search(ctx, chatgptsearch.BuildRequest(chatgptsearch.Options{
		Query:    *query,
		Limit:    *limit,
		Author:   *author,
		ThreadID: *threadID,
		From:     *from,
		To:       *to,
	}))
	if err != nil {
		return err
	}
	switch *format {
	case "json":
		return json.NewEncoder(stdout).Encode(resp)
	case "yaml", "yml":
		_, err := io.WriteString(stdout, formatYAML(resp))
		return err
	default:
		return fmt.Errorf("unsupported --format value: %s", *format)
	}
}

func formatYAML(resp any) string {
	raw, err := json.Marshal(resp)
	if err == nil {
		var decoded any
		if json.Unmarshal(raw, &decoded) == nil {
			var b strings.Builder
			writeYAMLValue(&b, 0, decoded)
			return b.String()
		}
	}
	var b strings.Builder
	writeYAMLValue(&b, 0, resp)
	return b.String()
}

func writeYAMLValue(b *strings.Builder, indent int, v any) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			writeIndent(b, indent)
			b.WriteString(k)
			b.WriteString(":")
			if isScalar(x[k]) {
				b.WriteString(" ")
				b.WriteString(formatYAMLScalar(x[k]))
				b.WriteString("\n")
				continue
			}
			b.WriteString("\n")
			writeYAMLValue(b, indent+2, x[k])
		}
	case []any:
		for _, item := range x {
			writeYAMLListItem(b, indent, item)
		}
	case []map[string]any:
		for _, item := range x {
			writeYAMLListItem(b, indent, item)
		}
	default:
		writeIndent(b, indent)
		b.WriteString(formatYAMLScalar(x))
		b.WriteString("\n")
	}
}

func writeYAMLListItem(b *strings.Builder, indent int, item any) {
	writeIndent(b, indent)
	b.WriteString("-")
	if isScalar(item) {
		b.WriteString(" ")
		b.WriteString(formatYAMLScalar(item))
		b.WriteString("\n")
		return
	}
	b.WriteString("\n")
	writeYAMLValue(b, indent+2, item)
}

func writeIndent(b *strings.Builder, indent int) {
	for range indent {
		b.WriteByte(' ')
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case nil, string, bool, float64, float32, int, int64, int32, uint64, uint32, uint:
		return true
	default:
		return false
	}
}

func formatYAMLScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		if x == "" {
			return `""`
		}
		if strings.Contains(x, "\n") {
			lines := strings.Split(x, "\n")
			var b strings.Builder
			b.WriteString("|-\n")
			for _, line := range lines {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			return b.String()
		}
		return strconv.Quote(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	default:
		return strconv.Quote(fmt.Sprintf("%v", x))
	}
}
