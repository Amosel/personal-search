package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"personal-search/internal/chatgptsearch"
	"personal-search/internal/model"
)

const (
	toolName        = "search_documents"
	protocolVersion = "2024-11-05"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  map[string]any `json:"result,omitempty"`
	Error   *rpcError      `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Handler struct {
	searcher chatgptsearch.Searcher
}

func main() {
	var server string
	flag.StringVar(&server, "server", "http://127.0.0.1:8080", "Search server base URL")
	flag.Parse()

	h := Handler{
		searcher: &chatgptsearch.HTTPClient{BaseURL: server},
	}
	if err := serve(context.Background(), os.Stdin, os.Stdout, h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, in io.Reader, out io.Writer, h Handler) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	for {
		req, err := readRPC(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		resp := rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		result, err := h.handle(ctx, req)
		if err != nil {
			resp.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			resp.Result = result
		}
		if req.ID == nil {
			continue
		}
		if err := writeRPC(writer, resp); err != nil {
			return err
		}
	}
}

func (h Handler) handle(ctx context.Context, req rpcRequest) (map[string]any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "chatgpt-search-mcp",
				"version": "0.1.0",
			},
		}, nil
	case "notifications/initialized":
		return nil, nil
	case "tools/list":
		return map[string]any{
			"tools": []map[string]any{
				{
					"name":        toolName,
					"description": "Search indexed ChatGPT conversations",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query":     map[string]any{"type": "string"},
							"limit":     map[string]any{"type": "number"},
							"author":    map[string]any{"type": "string"},
							"thread_id": map[string]any{"type": "string"},
							"from":      map[string]any{"type": "string"},
							"to":        map[string]any{"type": "string"},
						},
						"required": []string{"query"},
					},
				},
			},
		}, nil
	case "tools/call":
		if h.searcher == nil {
			return nil, fmt.Errorf("searcher is required")
		}
		var params struct {
			Name      string `json:"name"`
			Arguments struct {
				Query    string   `json:"query"`
				Limit    int      `json:"limit"`
				Author   string   `json:"author"`
				ThreadID string   `json:"thread_id"`
				From     string   `json:"from"`
				To       string   `json:"to"`
				Keywords []string `json:"keywords"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if params.Name != toolName {
			return nil, fmt.Errorf("unknown tool: %s", params.Name)
		}
		searchReq := chatgptsearch.BuildRequest(chatgptsearch.Options{
			Query:    params.Arguments.Query,
			Limit:    params.Arguments.Limit,
			Author:   params.Arguments.Author,
			ThreadID: params.Arguments.ThreadID,
			From:     params.Arguments.From,
			To:       params.Arguments.To,
		})
		if len(params.Arguments.Keywords) > 0 {
			searchReq.Filters.Keywords = params.Arguments.Keywords
		}
		out, err := h.searcher.Search(ctx, searchReq)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Found %d ChatGPT results", len(out.Results)),
				},
			},
			"structuredContent": out,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

func readRPC(r *bufio.Reader) (rpcRequest, error) {
	var req rpcRequest
	header, err := r.ReadString('\n')
	if err != nil {
		return req, err
	}
	if !strings.HasPrefix(strings.ToLower(header), "content-length:") {
		return req, fmt.Errorf("unexpected header: %q", header)
	}
	if _, err := r.ReadString('\n'); err != nil {
		return req, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:")))
	if err != nil {
		return req, fmt.Errorf("invalid content length: %w", err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return req, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	return req, nil
}

func writeRPC(w *bufio.Writer, resp rpcResponse) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		return err
	}
	return w.Flush()
}

var _ chatgptsearch.Searcher = (*chatgptsearch.HTTPClient)(nil)
var _ = model.SearchRequest{}
