package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClient_NilHTTPClient(t *testing.T) {
	t.Parallel()
	c, err := NewClient(nil, "")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.NotNil(t, c.gh)
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	// Empty baseURL and the literal public URL should both produce a client without error.
	for _, base := range []string{"", "https://api.github.com"} {
		c, err := NewClient(nil, base)
		require.NoError(t, err, "baseURL=%q", base)
		require.NotNil(t, c, "baseURL=%q", base)
	}
}

func TestNewClient_CustomBaseURL(t *testing.T) {
	t.Parallel()
	c, err := NewClient(nil, "https://ghes.example.com/api/v3")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, "https://ghes.example.com/api/v3/", c.gh.BaseURL.String())
}

func TestNewClient_DoesNotMutateCallerClient(t *testing.T) {
	t.Parallel()
	original := &http.Client{}
	_, err := NewClient(original, "")
	require.NoError(t, err)
	require.Nil(t, original.Transport, "caller's http.Client.Transport should not be mutated")
}

func TestWithGithubToken_AppliesAuthHeader(t *testing.T) {
	t.Parallel()

	var captured http.Header
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	c := &Client{transport: inner}
	ctx := WithGithubToken(context.Background(), "my-secret-token")

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos", nil)
	_, err := c.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, "token my-secret-token", captured.Get("Authorization"))
}

func TestClient_RoundTrip_NoAuthProvider(t *testing.T) {
	t.Parallel()

	var captured http.Header
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	c := &Client{transport: inner}
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos", nil)
	_, err := c.RoundTrip(req)
	require.NoError(t, err)
	require.Empty(t, captured.Get("Authorization"))
}

// roundTripFunc adapts a plain function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
