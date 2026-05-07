package handlers_test

import (
	"testing"
	"time"

	"github.com/plan42-ai/github-event-handlers/handlers"
	"github.com/stretchr/testify/require"
)

const (
	testOrg             = "myorg"
	testRepoName        = "myrepo"
	testFullName        = "myorg/myrepo"
	testLogin           = "alice"
	testStateOpen       = "open"
	testActionCreated   = "created"
	testActionSubmitted = "submitted"
	testActionOpened    = "opened"
	testDeliveryID      = "d-1"
)

func testRepository() handlers.Repository {
	return handlers.Repository{
		FullName: testFullName,
		Name:     testRepoName,
		Org:      testOrg,
	}
}

// Compile-time checks: every concrete event type must satisfy the Event interface.
var (
	_ handlers.Event = (*handlers.InstallationEvent)(nil)
	_ handlers.Event = (*handlers.IssueCommentEvent)(nil)
	_ handlers.Event = (*handlers.PullRequestReviewCommentEvent)(nil)
	_ handlers.Event = (*handlers.PullRequestReviewEvent)(nil)
	_ handlers.Event = (*handlers.PullRequestEvent)(nil)
)

func TestEventBase_DeliveryID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		deliveryID string
	}{
		{"non-empty", "delivery-42"},
		{"empty", ""},
		{"uuid-shaped", "550e8400-e29b-41d4-a716-446655440000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := handlers.EventBase{DeliveryID: tt.deliveryID}
			require.Equal(t, tt.deliveryID, base.GetDeliveryID())
		})
	}
}

func TestEventTypes(t *testing.T) {
	t.Parallel()

	now := time.Now()
	body := "review body"

	tests := []struct {
		name     string
		event    handlers.Event
		wantType string
		wantID   string
	}{
		{
			name: "InstallationEvent",
			event: &handlers.InstallationEvent{
				EventBase: handlers.EventBase{DeliveryID: testDeliveryID},
				Action:    testActionCreated,
				Installation: handlers.Installation{
					ID:       100,
					AppID:    200,
					AppSlug:  "plan42",
					OrgLogin: testOrg,
					OrgID:    300,
				},
			},
			wantType: "installation",
			wantID:   testDeliveryID,
		},
		{
			name: "IssueCommentEvent",
			event: &handlers.IssueCommentEvent{
				EventBase: handlers.EventBase{DeliveryID: "d-2"},
				Action:    testActionCreated,
				Issue: handlers.Issue{
					Number:        42,
					State:         testStateOpen,
					IsPullRequest: true,
				},
				Comment: handlers.Comment{
					Body:  "/Plan42",
					Login: testLogin,
				},
				Repository: testRepository(),
			},
			wantType: "issue_comment",
			wantID:   "d-2",
		},
		{
			name: "PullRequestReviewCommentEvent",
			event: &handlers.PullRequestReviewCommentEvent{
				EventBase: handlers.EventBase{DeliveryID: "d-3"},
				Action:    testActionCreated,
				Comment: handlers.Comment{
					Body:  "inline comment",
					Login: "bob",
				},
				PullRequest: handlers.PullRequest{
					ID:        1001,
					Number:    7,
					State:     testStateOpen,
					HTMLURL:   "https://github.com/myorg/myrepo/pull/7",
					UpdatedAt: &now,
					Login:     testLogin,
				},
				Repository: testRepository(),
			},
			wantType: "pull_request_review_comment",
			wantID:   "d-3",
		},
		{
			name: "PullRequestReviewEvent",
			event: &handlers.PullRequestReviewEvent{
				EventBase: handlers.EventBase{DeliveryID: "d-4"},
				Action:    testActionSubmitted,
				Review: handlers.Review{
					Body:  &body,
					Login: "charlie",
				},
				PullRequest: handlers.PullRequest{
					ID:     1002,
					Number: 8,
					State:  testStateOpen,
					Login:  testLogin,
				},
				Repository: testRepository(),
			},
			wantType: "pull_request_review",
			wantID:   "d-4",
		},
		{
			name: "PullRequestEvent",
			event: &handlers.PullRequestEvent{
				EventBase: handlers.EventBase{DeliveryID: "d-5"},
				Action:    testActionOpened,
				Number:    9,
				PullRequest: handlers.PullRequest{
					ID:      1003,
					Number:  9,
					State:   testStateOpen,
					Merged:  false,
					Draft:   true,
					HTMLURL: "https://github.com/myorg/myrepo/pull/9",
					Login:   testLogin,
				},
				Repository: testRepository(),
			},
			wantType: "pull_request",
			wantID:   "d-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.wantType, tt.event.EventType())
			require.Equal(t, tt.wantID, tt.event.GetDeliveryID())
		})
	}
}

func TestPullRequestReviewEvent_NilBody(t *testing.T) {
	t.Parallel()
	evt := &handlers.PullRequestReviewEvent{
		EventBase: handlers.EventBase{DeliveryID: "d-nil-body"},
		Action:    testActionSubmitted,
		Review: handlers.Review{
			Body:  nil,
			Login: "reviewer",
		},
	}
	require.Nil(t, evt.Review.Body)
}
