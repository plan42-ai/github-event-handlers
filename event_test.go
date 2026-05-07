package githubevents_test

import (
	"testing"
	"time"

	githubevents "github.com/plan42-ai/github-event-handlers"
)

const (
	testOrg           = "myorg"
	testRepoName      = "myrepo"
	testFullName      = "myorg/myrepo"
	testLogin         = "alice"
	testStateOpen     = "open"
	testActionCreated = "created"
	testDeliveryID    = "d-1"
)

func testRepository() githubevents.Repository {
	return githubevents.Repository{
		FullName: testFullName,
		Name:     testRepoName,
		Org:      testOrg,
	}
}

// Compile-time checks: every concrete event type must satisfy the Event interface.
var (
	_ githubevents.Event = (*githubevents.InstallationEvent)(nil)
	_ githubevents.Event = (*githubevents.IssueCommentEvent)(nil)
	_ githubevents.Event = (*githubevents.PullRequestReviewCommentEvent)(nil)
	_ githubevents.Event = (*githubevents.PullRequestReviewEvent)(nil)
	_ githubevents.Event = (*githubevents.PullRequestEvent)(nil)
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
			base := githubevents.EventBase{DeliveryID: tt.deliveryID}
			if got := base.GetDeliveryID(); got != tt.deliveryID {
				t.Errorf("GetDeliveryID() = %q, want %q", got, tt.deliveryID)
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	t.Parallel()

	now := time.Now()
	body := "review body"

	tests := []struct {
		name     string
		event    githubevents.Event
		wantType string
		wantID   string
	}{
		{
			name: "InstallationEvent",
			event: &githubevents.InstallationEvent{
				EventBase: githubevents.EventBase{DeliveryID: testDeliveryID},
				Action:    testActionCreated,
				Installation: githubevents.Installation{
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
			event: &githubevents.IssueCommentEvent{
				EventBase: githubevents.EventBase{DeliveryID: "d-2"},
				Action:    testActionCreated,
				Issue: githubevents.Issue{
					Number:        42,
					State:         testStateOpen,
					IsPullRequest: true,
				},
				Comment: githubevents.Comment{
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
			event: &githubevents.PullRequestReviewCommentEvent{
				EventBase: githubevents.EventBase{DeliveryID: "d-3"},
				Action:    testActionCreated,
				Comment: githubevents.Comment{
					Body:  "inline comment",
					Login: "bob",
				},
				PullRequest: githubevents.PullRequest{
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
			event: &githubevents.PullRequestReviewEvent{
				EventBase: githubevents.EventBase{DeliveryID: "d-4"},
				Action:    "submitted",
				Review: githubevents.Review{
					Body:  &body,
					Login: "charlie",
				},
				PullRequest: githubevents.PullRequest{
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
			event: &githubevents.PullRequestEvent{
				EventBase: githubevents.EventBase{DeliveryID: "d-5"},
				Action:    "opened",
				Number:    9,
				PullRequest: githubevents.PullRequest{
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
			if got := tt.event.EventType(); got != tt.wantType {
				t.Errorf("EventType() = %q, want %q", got, tt.wantType)
			}
			if got := tt.event.GetDeliveryID(); got != tt.wantID {
				t.Errorf("GetDeliveryID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestPullRequestReviewEvent_NilBody(t *testing.T) {
	t.Parallel()
	evt := &githubevents.PullRequestReviewEvent{
		EventBase: githubevents.EventBase{DeliveryID: "d-nil-body"},
		Action:    "submitted",
		Review: githubevents.Review{
			Body:  nil,
			Login: "reviewer",
		},
	}
	if evt.Review.Body != nil {
		t.Error("expected Review.Body to be nil")
	}
}
