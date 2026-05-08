package github

import (
	"net/http"
	"testing"
)

func TestNewClient_NilHTTPClient(t *testing.T) {
	t.Parallel()
	c, err := NewClient(nil, "tok", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.gh == nil {
		t.Fatal("expected non-nil go-github client")
	}
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	// Empty baseURL and the literal public URL should both produce a client without error.
	for _, base := range []string{"", "https://api.github.com"} {
		c, err := NewClient(nil, "tok", base)
		if err != nil {
			t.Fatalf("baseURL=%q: unexpected error: %v", base, err)
		}
		if c == nil {
			t.Fatalf("baseURL=%q: expected non-nil client", base)
		}
	}
}

func TestNewClient_CustomBaseURL(t *testing.T) {
	t.Parallel()
	c, err := NewClient(nil, "tok", "https://ghes.example.com/api/v3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	// The go-github client should be retargeted at the GHES instance.
	got := c.gh.BaseURL.String()
	want := "https://ghes.example.com/api/v3/"
	if got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
}

func TestNewClient_DoesNotMutateCallerClient(t *testing.T) {
	t.Parallel()
	original := &http.Client{}
	_, err := NewClient(original, "tok", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The original client's Transport should remain nil (not mutated).
	if original.Transport != nil {
		t.Errorf("caller's http.Client.Transport was mutated, got %T", original.Transport)
	}
}

func TestAuthTransport_SetsHeader(t *testing.T) {
	t.Parallel()

	var captured http.Header
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	at := &authTransport{wrapped: inner, token: "my-secret-token"}
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos", nil)

	_, err := at.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "token my-secret-token"
	if got := captured.Get("Authorization"); got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

// roundTripFunc adapts a plain function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
