package githubclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v81/github"
)

// GithubAPI is the handler-facing interface satisfied by *GithubClient. It exists for
// dependency injection in handler tests; the shared library provides one implementation.
// Methods take only the operation's intrinsic arguments; auth is applied by the
// authTransport on the underlying http client.
type GithubAPI interface {
	FindIssueCommentWithMarker(ctx context.Context, owner, repo string, issueNumber int, marker string) (*IssueComment, error)
	CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*IssueComment, error)
	UpdateIssueComment(ctx context.Context, owner, repo string, commentID int64, body string) (*IssueComment, error)
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)
}

// IssueComment represents a GitHub issue or PR comment.
type IssueComment struct {
	ID   int64
	Body string
}

// PullRequest represents a GitHub pull request as returned by GetPullRequest.
type PullRequest struct {
	ID    int64
	State string
	User  PullRequestUser
}

// PullRequestUser carries the login of a pull request author.
type PullRequestUser struct {
	Login string
}

// GithubClient is the GitHub API client used by handlers. It is built on top of go-github,
// authenticated with a static token via authTransport.
type GithubClient struct {
	gh *github.Client
}

// Compile-time check that *GithubClient satisfies GithubAPI.
var _ GithubAPI = (*GithubClient)(nil)

func coalesce[T comparable](l, r T) T {
	var zero T
	if l != zero {
		return l
	}
	return r
}

// NewGithubClient returns a GithubClient authenticated with the supplied token, issuing
// requests against baseURL using httpClient's transport.
//
// A nil httpClient falls back to http.DefaultClient. The caller's *http.Client is not
// mutated; we shallow-copy before installing the auth transport wrapper.
//
// baseURL "" or "https://api.github.com" targets public GitHub. Any other value retargets
// the underlying go-github client at a GHES instance via WithEnterpriseURLs.
func NewGithubClient(httpClient *http.Client, token, baseURL string) (*GithubClient, error) {
	ret := *coalesce(httpClient, http.DefaultClient)
	ret.Transport = &authTransport{
		wrapped: coalesce(ret.Transport, http.DefaultTransport),
		token:   token,
	}
	gh := github.NewClient(&ret)

	if baseURL != "" && baseURL != "https://api.github.com" {
		var err error
		gh, err = gh.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, err
		}
	}

	return &GithubClient{gh: gh}, nil
}

// authTransport wraps an http.RoundTripper and injects "Authorization: token <token>"
// on every outbound request.
type authTransport struct {
	wrapped http.RoundTripper
	token   string
}

func (at *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "token "+at.token)
	return at.wrapped.RoundTrip(req)
}

// FindIssueCommentWithMarker lists issue comments and returns the first one whose body
// contains the marker string. Returns nil, nil if no matching comment is found.
func (c *GithubClient) FindIssueCommentWithMarker(ctx context.Context, owner, repo string, issueNumber int, marker string) (*IssueComment, error) {
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		comments, resp, err := c.gh.Issues.ListComments(ctx, owner, repo, issueNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("list issue comments: %w", err)
		}

		for _, comment := range comments {
			if strings.Contains(comment.GetBody(), marker) {
				return &IssueComment{
					ID:   comment.GetID(),
					Body: comment.GetBody(),
				}, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return nil, nil
}

// CreateIssueComment creates a comment on the specified issue or pull request.
func (c *GithubClient) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*IssueComment, error) {
	comment, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, issueNumber, &github.IssueComment{
		Body: github.Ptr(body),
	})
	if err != nil {
		return nil, fmt.Errorf("create issue comment: %w", err)
	}
	return &IssueComment{
		ID:   comment.GetID(),
		Body: comment.GetBody(),
	}, nil
}

// UpdateIssueComment updates an existing issue comment by ID.
func (c *GithubClient) UpdateIssueComment(ctx context.Context, owner, repo string, commentID int64, body string) (*IssueComment, error) {
	comment, _, err := c.gh.Issues.EditComment(ctx, owner, repo, commentID, &github.IssueComment{
		Body: github.Ptr(body),
	})
	if err != nil {
		return nil, fmt.Errorf("update issue comment: %w", err)
	}
	return &IssueComment{
		ID:   comment.GetID(),
		Body: comment.GetBody(),
	}, nil
}

// GetPullRequest retrieves a pull request by number.
func (c *GithubClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get pull request: %w", err)
	}
	return &PullRequest{
		ID:    pr.GetID(),
		State: pr.GetState(),
		User: PullRequestUser{
			Login: pr.GetUser().GetLogin(),
		},
	}, nil
}
