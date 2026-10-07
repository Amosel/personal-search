package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

// ValidateEmbedding fails closed for legacy/unmarked or foreign-source points.
// Scan every payload: checking one record cannot detect mixed collections.
func (c *Client) ValidateEmbedding(ctx context.Context, collection, identity string, dim int) error {
	var config struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/collections/"+collection, nil, &config); err != nil {
		return err
	}
	if config.Result.Config.Params.Vectors.Size != dim || config.Result.Config.Params.Vectors.Distance != "Cosine" {
		return fmt.Errorf("collection %s requires matching dimension and Cosine distance", collection)
	}
	var offset any
	for {
		page, err := c.Scroll(ctx, collection, ScrollRequest{Limit: 256, Offset: offset, WithPayload: true})
		if err != nil {
			return err
		}
		for _, p := range page.Result.Points {
			if p.Payload["embedding_identity"] != identity || p.Payload["source"] != "google_search" {
				return fmt.Errorf("collection %s contains incompatible or unmarked embeddings; use a new dedicated collection", collection)
			}
		}
		offset = page.Result.NextPageOffset
		if offset == nil {
			return nil
		}
		if len(page.Result.Points) == 0 {
			return fmt.Errorf("empty embedding validation page with continuation")
		}
	}
}
