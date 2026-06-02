package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/go-github/v81/github"
	"github.com/stretchr/testify/require"
)

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

	evt, err := ParseEventsAPI(env)
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

	evt, err := ParseEventsAPI(env)
	require.NoError(t, err)
	require.NotNil(t, evt)
	require.Equal(t, "pull_request", evt.EventType())

	pr := evt.(*PullRequestEvent)
	require.Equal(t, "opened", pr.Action)
	require.Equal(t, int64(999), pr.PullRequest.ID)
	require.Equal(t, "bob", pr.PullRequest.Login)
	require.Equal(t, "acme", pr.Repository.Org)
	require.Equal(t, "widget", pr.Repository.Name)
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

	evt, err := ParseEventsAPI(env)
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

	evt, err := ParseEventsAPI(env)
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

	evt, err := ParseEventsAPI(env)
	require.NoError(t, err)
	require.NotNil(t, evt)

	review := evt.(*PullRequestReviewEvent)
	require.Equal(t, "dismissed", review.Action, "non-created actions should pass through")
}
