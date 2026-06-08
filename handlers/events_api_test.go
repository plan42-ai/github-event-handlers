package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/go-github/v81/github"
	ghapi "github.com/plan42-ai/github-event-handlers/github"
	"github.com/stretchr/testify/require"
)

// fakeGithubAPI is a minimal ghapi.API used by the Events API parser tests. Only
// GetPullRequest is exercised; the other methods satisfy the interface.
type fakeGithubAPI struct {
	pullRequestID      int64
	pullRequestNumber  int
	pullRequestState   string
	pullRequestAuthor  string
	pullRequestMerged  bool
	pullRequestDraft   bool
	pullRequestHTMLURL string
	getPRCalled        bool
}

func (f *fakeGithubAPI) GetPullRequest(_ context.Context, _, _ string, _ int) (*ghapi.PullRequest, error) {
	f.getPRCalled = true
	return &ghapi.PullRequest{
		ID:      f.pullRequestID,
		Number:  f.pullRequestNumber,
		State:   f.pullRequestState,
		Merged:  f.pullRequestMerged,
		Draft:   f.pullRequestDraft,
		HTMLURL: f.pullRequestHTMLURL,
		User:    ghapi.PullRequestUser{Login: f.pullRequestAuthor},
	}, nil
}

func (f *fakeGithubAPI) FindIssueCommentWithMarker(context.Context, string, string, int, string) (*ghapi.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) CreateIssueComment(context.Context, string, string, int, string) (*ghapi.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) UpdateIssueComment(context.Context, string, string, int64, string) (*ghapi.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) GetInstallationToken(context.Context, int64) (string, error) {
	return "", nil
}

func rawJSON(v any) *json.RawMessage {
	data, _ := json.Marshal(v)
	raw := json.RawMessage(data)
	return &raw
}

func TestParseEventsAPIIssueComment(t *testing.T) {
	payload := &github.IssueCommentEvent{
		Action: github.Ptr("created"),
		Comment: &github.IssueComment{
			Body: github.Ptr("hello"),
			User: &github.User{Login: github.Ptr("alice")},
		},
		Issue: &github.Issue{
			Number: github.Ptr(42),
		},
	}
	env := &github.Event{
		Type:       github.Ptr("IssueCommentEvent"),
		RawPayload: rawJSON(payload),
		Repo: &github.Repository{
			Name: github.Ptr("myorg/myrepo"),
		},
	}

	evt, err := ParseEventsAPI(context.Background(), env, &fakeGithubAPI{})
	require.NoError(t, err)
	require.NotNil(t, evt)
	require.Equal(t, "issue_comment", evt.EventType())
	require.NotEmpty(t, evt.GetDeliveryID())

	ic := evt.(*IssueCommentEvent)
	require.Equal(t, "created", ic.Action)
	require.Equal(t, "hello", ic.Comment.Body)
	require.Equal(t, "alice", ic.Comment.Login)
	require.Equal(t, "myorg/myrepo", ic.Repository.FullName)
	require.Equal(t, "myorg", ic.Repository.Org)
	require.Equal(t, "myrepo", ic.Repository.Name)
}

func TestParseEventsAPIPullRequest(t *testing.T) {
	payload := &github.PullRequestEvent{
		Action: github.Ptr("opened"),
		Number: github.Ptr(10),
		PullRequest: &github.PullRequest{
			ID:     github.Ptr(int64(999)),
			Number: github.Ptr(10),
			State:  github.Ptr("open"),
			User:   &github.User{Login: github.Ptr("bob")},
		},
	}
	env := &github.Event{
		Type:       github.Ptr("PullRequestEvent"),
		RawPayload: rawJSON(payload),
		Repo: &github.Repository{
			Name: github.Ptr("acme/widget"),
		},
	}

	// The Events API payload omits merged/draft/html_url/updated_at and the
	// author login, so the parser back-fills them from a GitHub PR fetch.
	gh := &fakeGithubAPI{
		pullRequestID:      999,
		pullRequestNumber:  10,
		pullRequestState:   "closed",
		pullRequestAuthor:  "bob",
		pullRequestMerged:  true,
		pullRequestDraft:   false,
		pullRequestHTMLURL: "https://github.com/acme/widget/pull/10",
	}

	evt, err := ParseEventsAPI(context.Background(), env, gh)
	require.NoError(t, err)
	require.NotNil(t, evt)
	require.Equal(t, "pull_request", evt.EventType())
	require.True(t, gh.getPRCalled, "parser should fetch the full PR to back-fill fields")

	pr := evt.(*PullRequestEvent)
	require.Equal(t, "opened", pr.Action)
	require.Equal(t, int64(999), pr.PullRequest.ID)
	require.Equal(t, "bob", pr.PullRequest.Login)
	require.Equal(t, "acme", pr.Repository.Org)
	require.Equal(t, "widget", pr.Repository.Name)
	// Fields back-filled from the fetched PR, not present in the Events API payload.
	require.True(t, pr.PullRequest.Merged)
	require.Equal(t, "https://github.com/acme/widget/pull/10", pr.PullRequest.HTMLURL)
}

func TestParseEventsAPIUnsupportedType(t *testing.T) {
	// Use a type the Events API delivers but we don't handle.
	payload := &github.WatchEvent{
		Action: github.Ptr("started"),
	}
	env := &github.Event{
		Type:       github.Ptr("WatchEvent"),
		RawPayload: rawJSON(payload),
		Repo: &github.Repository{
			Name: github.Ptr("org/repo"),
		},
	}

	evt, err := ParseEventsAPI(context.Background(), env, &fakeGithubAPI{})
	require.NoError(t, err)
	require.Nil(t, evt, "unsupported types should return nil, nil")
}

func TestEventsAPIRepository(t *testing.T) {
	env := &github.Event{
		Repo: &github.Repository{
			Name: github.Ptr("myorg/myrepo"),
		},
	}

	repo := eventsAPIRepository(env)
	require.Equal(t, "myorg/myrepo", repo.FullName)
	require.Equal(t, "myorg", repo.Org)
	require.Equal(t, "myrepo", repo.Name)
}

func TestParseEventsAPIReviewNormalizesAction(t *testing.T) {
	// The Events API delivers new reviews with action "created", but the
	// shared handler expects "submitted". Verify normalization.
	payload := &github.PullRequestReviewEvent{
		Action: github.Ptr("created"),
		Review: &github.PullRequestReview{
			Body: github.Ptr("looks good"),
			User: &github.User{Login: github.Ptr("reviewer")},
		},
		PullRequest: &github.PullRequest{
			ID:     github.Ptr(int64(42)),
			Number: github.Ptr(7),
			State:  github.Ptr("open"),
			User:   &github.User{Login: github.Ptr("author")},
		},
	}
	env := &github.Event{
		Type:       github.Ptr("PullRequestReviewEvent"),
		RawPayload: rawJSON(payload),
		Repo: &github.Repository{
			Name: github.Ptr("org/repo"),
		},
	}

	evt, err := ParseEventsAPI(context.Background(), env, &fakeGithubAPI{})
	require.NoError(t, err)
	require.NotNil(t, evt)

	review := evt.(*PullRequestReviewEvent)
	require.Equal(t, "submitted", review.Action, "action should be normalized from 'created' to 'submitted'")
	require.Equal(t, "looks good", *review.Review.Body)
	require.Equal(t, "reviewer", review.Review.Login)
}

func TestParseEventsAPIReviewPreservesOtherActions(t *testing.T) {
	// Non-"created" actions should be passed through unchanged.
	payload := &github.PullRequestReviewEvent{
		Action: github.Ptr("dismissed"),
		Review: &github.PullRequestReview{
			User: &github.User{Login: github.Ptr("reviewer")},
		},
		PullRequest: &github.PullRequest{
			ID:     github.Ptr(int64(42)),
			Number: github.Ptr(7),
			State:  github.Ptr("open"),
			User:   &github.User{Login: github.Ptr("author")},
		},
	}
	env := &github.Event{
		Type:       github.Ptr("PullRequestReviewEvent"),
		RawPayload: rawJSON(payload),
		Repo: &github.Repository{
			Name: github.Ptr("org/repo"),
		},
	}

	evt, err := ParseEventsAPI(context.Background(), env, &fakeGithubAPI{})
	require.NoError(t, err)
	require.NotNil(t, evt)

	review := evt.(*PullRequestReviewEvent)
	require.Equal(t, "dismissed", review.Action, "non-created actions should pass through")
}
