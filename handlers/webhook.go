package handlers

import "github.com/google/go-github/v81/github"

func ParseWebhook(deliveryID string, messageType string, payload []byte) (Event, error) {
	event, err := github.ParseWebHook(messageType, payload)
	if err != nil {
		return nil, err
	}

	switch event := event.(type) {
	case *github.InstallationEvent:
		return webhookToInstallation(deliveryID, event), nil
	case *github.IssueCommentEvent:
		return webhookToIssueComment(deliveryID, event), nil
	case *github.PullRequestReviewCommentEvent:
		return webhookToReviewComment(deliveryID, event), nil
	case *github.PullRequestReviewEvent:
		return webhookToReview(deliveryID, event), nil
	default:
		return nil, ErrUnknownEvent
	}
}

func webhookToInstallation(deliveryID string, evt *github.InstallationEvent) *InstallationEvent {
	return &InstallationEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    evt.GetAction(),
		Installation: Installation{
			ID:       evt.GetInstallation().GetID(),
			AppID:    evt.GetInstallation().GetAppID(),
			AppSlug:  evt.GetInstallation().GetAppSlug(),
			OrgLogin: evt.GetInstallation().GetAccount().GetLogin(),
			OrgID:    evt.GetInstallation().GetAccount().GetID(),
		},
	}
}



func webhookToIssueComment(deliveryID string, evt *github.IssueCommentEvent) *IssueCommentEvent {
	var installationID *int64
	if evt.Installation != nil {
		installationID = evt.Installation.ID
	}

	return &IssueCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    evt.GetAction(),
		Comment: Comment{
			Body:  evt.GetComment().GetBody(),
			Login: evt.GetComment().GetUser().GetLogin(),
		},
		Issue: Issue{
			Number:        evt.GetIssue().GetNumber(),
			State:         evt.GetIssue().GetState(),
			IsPullRequest: evt.GetIssue().IsPullRequest(),
		},
		Repository:     webhookRepository(evt.GetRepo()),
		InstallationID: installationID,
	}
}

func webhookToReviewComment(deliveryID string, evt *github.PullRequestReviewCommentEvent) *PullRequestReviewCommentEvent {
	return &PullRequestReviewCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    evt.GetAction(),
		Comment: Comment{
			Body:  evt.GetComment().GetBody(),
			Login: evt.GetComment().GetUser().GetLogin(),
		},
		PullRequest: webhookPullRequest(evt.GetPullRequest()),
		Repository:  webhookRepository(evt.GetRepo()),
	}
}

func webhookToReview(deliveryID string, evt *github.PullRequestReviewEvent) *PullRequestReviewEvent {
	var body *string
	if evt.GetReview().Body != nil {
		v := *evt.GetReview().Body
		body = &v
	}

	return &PullRequestReviewEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    evt.GetAction(),
		Review: Review{
			Body:  body,
			Login: evt.GetReview().GetUser().GetLogin(),
		},
		PullRequest: webhookPullRequest(evt.GetPullRequest()),
		Repository:  webhookRepository(evt.GetRepo()),
	}
}

func webhookRepository(repo *github.Repository) Repository {
	return Repository{
		FullName: repo.GetFullName(),
		Name:     repo.GetName(),
		Org:      repo.GetOwner().GetLogin(),
	}
}

func webhookPullRequest(pr *github.PullRequest) PullRequest {
	return PullRequest{
		ID:     pr.GetID(),
		Number: pr.GetNumber(),
		State:  pr.GetState(),
		Login:  pr.GetUser().GetLogin(),
	}
}
