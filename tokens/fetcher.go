package tokens

import (
	"context"
	"time"

	"github.com/plan42-ai/cache"
	"github.com/plan42-ai/github-event-handlers/github"
)

const defaultTTL = 55 * time.Minute

// Fetcher fetches and caches GitHub App installation tokens.
type Fetcher interface {
	InstallationToken(ctx context.Context, gh github.API, installationID int64) (string, error)
}

type fetcher struct {
	signer   github.JWTSigner
	appID    int64
	keyAlias string
	cache    *cache.Cache[int64, string]
}

// NewFetcher returns a Fetcher that caches installation tokens for the provided TTL.
func NewFetcher(signer github.JWTSigner, appID int64, keyAlias string, ttl time.Duration) Fetcher {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &fetcher{
		signer:   signer,
		appID:    appID,
		keyAlias: keyAlias,
		cache:    cache.NewCacheWithTTL[int64, string](ttl),
	}
}

func (f *fetcher) InstallationToken(ctx context.Context, gh github.API, installationID int64) (string, error) {
	if token, ok := f.cache.GetCachedValue(installationID); ok {
		return token, nil
	}

	ctx = github.WithGithubAppAuth(ctx, f.signer, f.appID, f.keyAlias)
	token, err := gh.GetInstallationToken(ctx, installationID)
	if err != nil {
		return "", err
	}

	stored := f.cache.AddIfNotPresent(installationID, token)
	return stored, nil
}
