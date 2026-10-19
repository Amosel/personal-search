package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

// ValidateEmbedding fails closed for legacy/unmarked or foreign-source points.
// Scan every payload: checking one record cannot detect mixed collections.
func (c *Client) ValidateEmbedding(ctx context.Context, collection, identity string, dim int) error {
	return c.ValidateSourceEmbedding(ctx, collection, identity, dim, "google_search")
}

// ValidateSourceEmbedding rejects mixed sources and incompatible embedding identities.
func (c *Client) ValidateSourceEmbedding(ctx context.Context, collection, identity string, dim int, source string) error {
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
	var pagination Pagination
	for {
		page, err := c.Scroll(ctx, collection, ScrollRequest{Limit: 256, Offset: pagination.Offset, WithPayload: true})
		if err != nil {
			return err
		}
		for _, p := range page.Result.Points {
			if p.Payload["embedding_identity"] != identity || p.Payload["source"] != source {
				return fmt.Errorf("collection %s contains incompatible or unmarked embeddings; use a new dedicated collection", collection)
			}
		}
		more, err := pagination.Advance(page)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}
