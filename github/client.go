package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/plan42-ai/openid/jwt"
)

// API is the handler-facing interface satisfied by *Client. It exists for
// dependency injection in handler tests; the shared library provides one implementation.
// Methods take only the operation's intrinsic arguments; auth is applied by the
// authTransport on the underlying http client.
type API interface {
	FindIssueCommentWithMarker(ctx context.Context, owner, repo string, issueNumber int, marker string) (*IssueComment, error)
	CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*IssueComment, error)
	UpdateIssueComment(ctx context.Context, owner, repo string, commentID int64, body string) (*IssueComment, error)
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)
	GetInstallationToken(ctx context.Context, installationID int64) (string, error)
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

// Client is the GitHub API client used by handlers. It is built on top of go-github,
// authenticated with a static token via authTransport.
type Client struct {
	transport http.RoundTripper
	gh        *github.Client
}

// Compile-time check that *Client satisfies API.
var _ API = (*Client)(nil)

func coalesce[T comparable](l, r T) T {
	var zero T
	if l != zero {
		return l
	}
	return r
}

type AuthProvider interface {
	AddAuth(req *http.Request) (*http.Request, error)
}

type contextKey string

var contextKeyAuthProvider = contextKey("authProvider")

type tokenAuthProvider struct {
	token string
}

func (th *tokenAuthProvider) AddAuth(req *http.Request) (*http.Request, error) {
	req.Header.Set("Authorization", "token "+th.token)
	return req, nil
}

// JWTSigner signs a GitHub App JWT using a KMS-backed key.
type JWTSigner interface {
	SignGithubJWT(ctx context.Context, token *jwt.Token, keyAlias string) error
}

func WithGithubToken(ctx context.Context, token string) context.Context {
	return context.WithValue(
		ctx,
		contextKeyAuthProvider,
		&tokenAuthProvider{
			token: token,
		},
	)
}

func GetAuthProvider(ctx context.Context) AuthProvider {
	authProvider, _ := ctx.Value(contextKeyAuthProvider).(AuthProvider)
	return authProvider
}

type jwtAuthProvider struct {
	signer   JWTSigner
	appID    int64
	keyAlias string
}

func (j *jwtAuthProvider) AddAuth(req *http.Request) (*http.Request, error) {
	ctx := req.Context()
	now := time.Now().UTC()
	iat := now.Add(-1 * time.Minute)
	exp := now.Add(8 * time.Minute)

	issStr := strconv.FormatInt(j.appID, 10)

	issURL, err := url.Parse(issStr)
	if err != nil {
		return nil, err
	}

	token := jwt.Token{
		Header: jwt.Header{
			Algorithm: jwt.AlgorithmRS256,
			Type:      "JWT",
		},
		Payload: jwt.Payload{
			Issuer:     issURL,
			Subject:    issStr,
			Audience:   "github",
			IssuedAt:   iat,
			NotBefore:  iat,
			Expiration: exp,
		},
	}

	if err := j.signer.SignGithubJWT(ctx, &token, j.keyAlias); err != nil {
		return nil, fmt.Errorf("failed to sign github app jwt: %w", err)
	}
	jwtString := token.String()
	req.Header.Add("Authorization", "bearer "+jwtString)
	return req, nil
}

func WithGithubAppAuth(ctx context.Context, signer JWTSigner, appID int64, keyAlias string) context.Context {
	return context.WithValue(
		ctx,
		contextKeyAuthProvider,
		&jwtAuthProvider{
			signer:   signer,
			appID:    appID,
			keyAlias: keyAlias,
		},
	)
}

// NewClient returns a Client authenticated with the supplied token, issuing
// requests against baseURL using httpClient's transport.
//
// A nil httpClient falls back to http.DefaultClient. The caller's *http.Client is not
// mutated; we shallow-copy before installing the auth transport wrapper.
//
// baseURL "" or "https://api.github.com" targets public GitHub. Any other value retargets
// the underlying go-github client at a GHES instance via WithEnterpriseURLs.
func NewClient(httpClient *http.Client, baseURL string) (*Client, error) {
	httpClient = new(*coalesce(httpClient, http.DefaultClient))
	ret := &Client{
		transport: coalesce(httpClient.Transport, http.DefaultTransport),
	}
	httpClient.Transport = ret
	ret.gh = github.NewClient(httpClient)

	if baseURL != "" && baseURL != "https://api.github.com" {
		var err error
		ret.gh, err = ret.gh.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (c *Client) RoundTrip(req *http.Request) (*http.Response, error) {
	ap := GetAuthProvider(req.Context())
	var err error
	if ap != nil {
		req, err = ap.AddAuth(req)
		if err != nil {
			return nil, err
		}
	}
	return c.transport.RoundTrip(req)
}

// FindIssueCommentWithMarker lists issue comments and returns the first one whose body
// contains the marker string. Returns nil, nil if no matching comment is found.
func (c *Client) FindIssueCommentWithMarker(ctx context.Context, owner, repo string, issueNumber int, marker string) (*IssueComment, error) {
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
func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*IssueComment, error) {
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
func (c *Client) UpdateIssueComment(ctx context.Context, owner, repo string, commentID int64, body string) (*IssueComment, error) {
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
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
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

func (c *Client) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	token, _, err := c.gh.Apps.CreateInstallationToken(ctx, installationID, &github.InstallationTokenOptions{})
	if err != nil {
		return "", fmt.Errorf("create installation token: %w", err)
	}
	return token.GetToken(), nil
}
