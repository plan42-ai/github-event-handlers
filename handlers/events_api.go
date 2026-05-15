package handlers

import (
	"strings"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/google/uuid"
)

// ParseEventsAPI translates a go-github Events API envelope into a shared
// library Event. It calls env.ParsePayload() internally, type-switches on
// the result, and returns the matching concrete Event value. Unsupported
// event types (including InstallationEvent, which the Events API does not
// deliver) return nil, nil so callers can skip them.
//
// A fresh random UUID is generated for the delivery ID, matching the format
// webhooks use for X-GitHub-Delivery.
func ParseEventsAPI(env *github.Event) (Event, error) {
	payload, err := env.ParsePayload()
	if err != nil {
		return nil, err
	}

	deliveryID := uuid.NewString()
	repo := eventsAPIRepository(env)

	switch p := payload.(type) {
	case *github.IssueCommentEvent:
		return eventsAPIToIssueComment(deliveryID, p, repo), nil
	case *github.PullRequestReviewCommentEvent:
		return eventsAPIToReviewComment(deliveryID, p, repo), nil
	case *github.PullRequestReviewEvent:
		return eventsAPIToReview(deliveryID, p, repo), nil
	case *github.PullRequestEvent:
		return eventsAPIToPullRequest(deliveryID, p, repo), nil
	default:
		// Unsupported event type; caller should skip.
		return nil, nil
	}
}

func eventsAPIToIssueComment(deliveryID string, p *github.IssueCommentEvent, repo Repository) *IssueCommentEvent {
	return &IssueCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Comment: Comment{
			Body:  p.GetComment().GetBody(),
			Login: p.GetComment().GetUser().GetLogin(),
		},
		Issue: Issue{
			Number:        p.GetIssue().GetNumber(),
			State:         p.GetIssue().GetState(),
			IsPullRequest: p.GetIssue().IsPullRequest(),
		},
		Repository: repo,
	}
}

func eventsAPIToReviewComment(deliveryID string, p *github.PullRequestReviewCommentEvent, repo Repository) *PullRequestReviewCommentEvent {
	return &PullRequestReviewCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Comment: Comment{
			Body:  p.GetComment().GetBody(),
			Login: p.GetComment().GetUser().GetLogin(),
		},
		PullRequest: eventsAPIPullRequest(p.GetPullRequest()),
		Repository:  repo,
	}
}

func eventsAPIToReview(deliveryID string, p *github.PullRequestReviewEvent, repo Repository) *PullRequestReviewEvent {
	var body *string
	if p.GetReview().Body != nil {
		v := *p.GetReview().Body
		body = &v
	}

	return &PullRequestReviewEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Review: Review{
			Body:  body,
			Login: p.GetReview().GetUser().GetLogin(),
		},
		PullRequest: eventsAPIPullRequest(p.GetPullRequest()),
		Repository:  repo,
	}
}

func eventsAPIToPullRequest(deliveryID string, p *github.PullRequestEvent, repo Repository) *PullRequestEvent {
	pr := p.GetPullRequest()

	var updatedAt *time.Time
	if ts := pr.UpdatedAt; ts != nil && !ts.IsZero() {
		t := ts.Time
		updatedAt = &t
	}

	return &PullRequestEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Number:    p.GetNumber(),
		PullRequest: PullRequest{
			ID:        pr.GetID(),
			Number:    pr.GetNumber(),
			State:     pr.GetState(),
			Merged:    pr.GetMerged(),
			Draft:     pr.GetDraft(),
			HTMLURL:   pr.GetHTMLURL(),
			UpdatedAt: updatedAt,
			Login:     pr.GetUser().GetLogin(),
		},
		Repository: repo,
	}
}

// eventsAPIRepository extracts Repository from the Events API envelope.
// The envelope's Repo.Name carries "owner/name"; FullName and Owner are nil.
func eventsAPIRepository(env *github.Event) Repository {
	name := env.GetRepo().GetName()
	org, repoName, _ := strings.Cut(name, "/")
	return Repository{
		FullName: name,
		Org:      org,
		Name:     repoName,
	}
}

// eventsAPIPullRequest builds a PullRequest from the Events API payload's PR.
func eventsAPIPullRequest(pr *github.PullRequest) PullRequest {
	return PullRequest{
		ID:     pr.GetID(),
		Number: pr.GetNumber(),
		State:  pr.GetState(),
		Login:  pr.GetUser().GetLogin(),
	}
}
