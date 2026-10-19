package googlesearch

import (
	"context"
	"personal-search/internal/embed"
)

// PinnedEmbedder is a compatibility name; provider identity belongs to embed.
type PinnedEmbedder = embed.PinnedOllama

func NewEmbedder(ctx context.Context, baseURL, model string) (*PinnedEmbedder, error) {
	return embed.NewPinnedOllama(ctx, baseURL, model, 0, nil)
}
