package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

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
	return json.NewEncoder(stdout).Encode(resp)
}
