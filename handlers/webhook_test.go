package handlers

import (
	"testing"

	github "github.com/google/go-github/v81/github"
	"github.com/stretchr/testify/require"
)

func TestWebhookToIssueCommentSetsInstallationID(t *testing.T) {
	installID := int64(4242)
	evt := &github.IssueCommentEvent{
		Action: github.Ptr("created"),
		Comment: &github.IssueComment{
			Body: github.Ptr("/plan42"),
			User: &github.User{Login: github.Ptr("alice")},
		},
		Issue: &github.Issue{
			Number:           github.Ptr(7),
			State:            github.Ptr("open"),
			PullRequestLinks: &github.PullRequestLinks{URL: github.Ptr("https://example.test")},
		},
		Repo: &github.Repository{
			FullName: github.Ptr("octo/demo"),
			Name:     github.Ptr("demo"),
			Owner:    &github.User{Login: github.Ptr("octo")},
		},
		Installation: &github.Installation{ID: github.Ptr(installID)},
	}

	result := webhookToIssueComment("delivery-install", evt)
	require.NotNil(t, result.InstallationID)
	require.Equal(t, installID, *result.InstallationID)
}

func TestWebhookToIssueCommentHandlesMissingInstallation(t *testing.T) {
	evt := &github.IssueCommentEvent{
		Action: github.Ptr("created"),
		Comment: &github.IssueComment{
			Body: github.Ptr("/plan42"),
			User: &github.User{Login: github.Ptr("alice")},
		},
		Issue: &github.Issue{
			Number:           github.Ptr(8),
			State:            github.Ptr("open"),
			PullRequestLinks: &github.PullRequestLinks{URL: github.Ptr("https://example.test")},
		},
		Repo: &github.Repository{
			FullName: github.Ptr("octo/demo"),
			Name:     github.Ptr("demo"),
			Owner:    &github.User{Login: github.Ptr("octo")},
		},
	}

	result := webhookToIssueComment("delivery-no-install", evt)
	require.Nil(t, result.InstallationID)
}
