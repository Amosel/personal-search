package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"personal-search/internal/chatgptsearch"
	"personal-search/internal/model"
)

func main() {
	query := flag.String("query", "", "Search query")
	server := flag.String("server", "http://127.0.0.1:8080", "Google Search server URL")
	from := flag.String("from", "", "Inclusive UTC date YYYY-MM-DD")
	to := flag.String("to", "", "Inclusive UTC date YYYY-MM-DD")
	limit := flag.Int("limit", 10, "Result limit")
	flag.Parse()
	req := model.SearchRequest{Query: *query, Limit: *limit, Filters: &model.Filters{Source: []string{"google_search"}}}
	if *from != "" || *to != "" {
		req.Filters.Date = &model.DateRange{From: *from, To: *to}
	}
	if err := req.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := chatgptsearch.HTTPClient{BaseURL: *server}
	result, err := client.Search(context.Background(), req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
