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
