package tokens

import (
	"context"
	"testing"

	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/openid/jwt"
	"github.com/stretchr/testify/require"
)

func TestFetcherCachesTokens(t *testing.T) {
	t.Parallel()

	gh := &fakeGithubAPI{token: "token-123"}
	signer := &fakeSigner{}
	fetcher := NewFetcher(signer, 42, "alias/plan42", 0)

	ctx := context.Background()

	token, err := fetcher.InstallationToken(ctx, gh, 123)
	require.NoError(t, err)
	require.Equal(t, "token-123", token)

	token, err = fetcher.InstallationToken(ctx, gh, 123)
	require.NoError(t, err)
	require.Equal(t, "token-123", token)
	require.Equal(t, 1, gh.calls, "token should have been fetched once")
}

type fakeGithubAPI struct {
	github.API
	token string
	calls int
}

func (f *fakeGithubAPI) FindIssueCommentWithMarker(context.Context, string, string, int, string) (*github.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) CreateIssueComment(context.Context, string, string, int, string) (*github.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) UpdateIssueComment(context.Context, string, string, int64, string) (*github.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) GetPullRequest(context.Context, string, string, int) (*github.PullRequest, error) {
	return nil, nil
}

func (f *fakeGithubAPI) GetInstallationToken(context.Context, int64) (string, error) {
	f.calls++
	return f.token, nil
}

type fakeSigner struct{}

func (f *fakeSigner) SignGithubJWT(context.Context, *jwt.Token, string) error {
	return nil
}

// Ensure fakeGithubAPI satisfies github.API.
var _ github.API = (*fakeGithubAPI)(nil)

// Ensure fakeSigner satisfies github.JWTSigner.
var _ github.JWTSigner = (*fakeSigner)(nil)
