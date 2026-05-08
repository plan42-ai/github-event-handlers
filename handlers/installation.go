package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/sdk-go/p42"
)

func newInstallationHandler(cfg Config) func(ctx context.Context, evt Event, gh github.API) {
	h := &installationHandler{
		appSlug: strings.TrimSpace(cfg.GithubAppName),
		appID:   cfg.GithubAppID,
		client:  cfg.Plan42Client,
	}
	return h.handle
}

type installationHandler struct {
	appSlug string
	appID   int64
	client  Plan42Client
}

func (h *installationHandler) handle(ctx context.Context, evt Event, _ github.API) {
	ie, ok := evt.(*InstallationEvent)
	if !ok {
		slog.ErrorContext(ctx, "installation handler received unexpected event type",
			"delivery_id", evt.GetDeliveryID(),
			"event_type", evt.EventType(),
		)
		return
	}

	deliveryID := ie.GetDeliveryID()

	slog.InfoContext(ctx, "received installation event",
		"delivery_id", deliveryID,
		"event", ie.EventType(),
	)

	appSlug := strings.TrimSpace(ie.Installation.AppSlug)
	if h.appSlug == "" {
		slog.WarnContext(ctx, "github app name not configured; skipping installation event",
			"delivery_id", deliveryID,
			"app_slug", appSlug,
		)
		return
	}
	if !strings.EqualFold(appSlug, h.appSlug) {
		slog.InfoContext(ctx, "installation event app slug does not match configured app; ignoring",
			"delivery_id", deliveryID,
			"app_slug", appSlug,
			"expected_app_slug", h.appSlug,
		)
		return
	}

	if h.appID <= 0 {
		slog.WarnContext(ctx, "github app id not configured; skipping installation event",
			"delivery_id", deliveryID,
			"app_slug", appSlug,
		)
		return
	}

	appID := ie.Installation.AppID
	if appID == 0 {
		slog.WarnContext(ctx, "installation event missing app id",
			"delivery_id", deliveryID,
			"app_slug", appSlug,
		)
		return
	}
	if appID != h.appID {
		slog.InfoContext(ctx, "installation event app id does not match configured app; ignoring",
			"delivery_id", deliveryID,
			"app_slug", appSlug,
			"app_id", appID,
			"configured_app_id", h.appID,
		)
		return
	}

	if h.client == nil {
		slog.WarnContext(ctx, "github client not configured; skipping installation update",
			"delivery_id", deliveryID,
		)
		return
	}

	login := strings.TrimSpace(ie.Installation.OrgLogin)
	if login == "" {
		slog.WarnContext(ctx, "installation event missing account login",
			"delivery_id", deliveryID,
		)
		return
	}

	action := strings.ToLower(strings.TrimSpace(ie.Action))
	externalID := ie.Installation.OrgID

	switch action {
	case "deleted":
		h.handleDeletion(ctx, deliveryID, login, externalID, ie.Installation.ID)
	case "created":
		h.handleCreation(ctx, deliveryID, login, externalID, ie.Installation.ID)
	default:
		slog.InfoContext(ctx, "installation action not supported; ignoring",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"action", action,
		)
	}
}

func (h *installationHandler) handleCreation(
	ctx context.Context,
	deliveryID,
	login string,
	externalID,
	installationID int64,
) {
	if installationID <= 0 {
		slog.WarnContext(ctx, "installation event missing installation ID",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
		)
		return
	}

	installation := int(installationID)
	if int64(installation) != installationID {
		slog.ErrorContext(ctx, "installation id overflows int",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"installation_id", installationID,
		)
		return
	}

	org, found, err := h.lookupGithubOrg(ctx, login, externalID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to look up github org",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"error", err,
		)
		return
	}

	if found {
		if org.InstallationID == installation {
			slog.InfoContext(ctx, "installation ID already up to date",
				"delivery_id", deliveryID,
				"login", login,
				"external_org_id", externalID,
				"installation_id", installationID,
			)
			return
		}

		if err := h.updateInstallationID(ctx, org, installation); err != nil {
			slog.ErrorContext(ctx, "failed to update installation ID",
				"delivery_id", deliveryID,
				"login", login,
				"external_org_id", externalID,
				"installation_id", installationID,
				"error", err,
			)
			return
		}

		slog.InfoContext(ctx, "installation ID updated",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"installation_id", installationID,
		)
		return
	}

	if err := h.addGithubOrg(ctx, login, externalID, installation); err != nil {
		slog.ErrorContext(ctx, "failed to add github org",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"installation_id", installationID,
			"error", err,
		)
		return
	}

	slog.InfoContext(ctx, "github org added",
		"delivery_id", deliveryID,
		"login", login,
		"external_org_id", externalID,
		"installation_id", installationID,
	)
}

func (h *installationHandler) handleDeletion(
	ctx context.Context,
	deliveryID,
	login string,
	externalID,
	installationID int64,
) {
	org, found, err := h.lookupGithubOrg(ctx, login, externalID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to look up github org",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"error", err,
		)
		return
	}

	if !found || org.Deleted {
		slog.InfoContext(ctx, "github org not found for installation deletion",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
		)
		return
	}

	// Guard against stale or replayed deletion events: only delete the org if the
	// installation ID on the event matches the one currently stored. If a newer
	// "created" event has already updated the org to a different installation, this
	// older "deleted" event must be a no-op.
	if installationID > 0 && org.InstallationID != int(installationID) {
		slog.InfoContext(ctx, "installation ID mismatch on deletion; ignoring stale event",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"event_installation_id", installationID,
			"stored_installation_id", org.InstallationID,
		)
		return
	}

	if err := h.deleteGithubOrg(ctx, org); err != nil {
		slog.ErrorContext(ctx, "failed to delete github org",
			"delivery_id", deliveryID,
			"login", login,
			"external_org_id", externalID,
			"error", err,
		)
		return
	}

	slog.InfoContext(ctx, "github org deleted",
		"delivery_id", deliveryID,
		"login", login,
		"external_org_id", externalID,
	)
}

func (h *installationHandler) lookupGithubOrg(ctx context.Context, login string, externalID int64) (
	*p42.GithubOrg,
	bool,
	error,
) {
	if h.client == nil {
		return nil, false, errors.New("github client is not configured")
	}

	nameFilter := login
	includeDeleted := true
	resp, err := h.client.ListGithubOrgs(ctx, &p42.ListGithubOrgsRequest{Name: &nameFilter, IncludeDeleted: &includeDeleted})
	if err != nil {
		return nil, false, fmt.Errorf("list github orgs: %w", err)
	}
	if resp == nil {
		return nil, false, errors.New("list github orgs returned nil response")
	}

	for i := range resp.Orgs {
		candidate := &resp.Orgs[i]
		if externalID != 0 && int64(candidate.ExternalOrgID) == externalID {
			return candidate, true, nil
		}
		if strings.EqualFold(candidate.OrgName, login) {
			return candidate, true, nil
		}
	}

	return nil, false, nil
}

func (h *installationHandler) addGithubOrg(
	ctx context.Context,
	login string,
	externalID int64,
	installation int,
) error {
	if h.client == nil {
		return errors.New("github client is not configured")
	}
	if login == "" {
		return errors.New("github org login is empty")
	}
	if externalID <= 0 {
		return errors.New("external org id is missing")
	}

	external := int(externalID)
	if int64(external) != externalID {
		return fmt.Errorf("external org id overflows int")
	}

	req := &p42.AddGithubOrgRequest{
		OrgID:          uuid.NewString(),
		OrgName:        login,
		ExternalOrgID:  external,
		InstallationID: installation,
	}

	if _, err := h.client.AddGithubOrg(ctx, req); err != nil {
		return fmt.Errorf("add github org: %w", err)
	}

	return nil
}

func (h *installationHandler) updateInstallationID(
	ctx context.Context,
	org *p42.GithubOrg,
	installation int,
) error {
	if h.client == nil {
		return errors.New("github client is not configured")
	}
	if org == nil {
		return errors.New("github org is nil")
	}

	version := org.Version
	deleted := false
	req := &p42.UpdateGithubOrgRequest{
		OrgID:          org.OrgID,
		Version:        version,
		Deleted:        &deleted,
		InstallationID: &installation,
	}

	if _, err := h.client.UpdateGithubOrg(ctx, req); err != nil {
		return fmt.Errorf("update github org: %w", err)
	}

	return nil
}

func (h *installationHandler) deleteGithubOrg(ctx context.Context, org *p42.GithubOrg) error {
	if h.client == nil {
		return errors.New("github client is not configured")
	}
	if org == nil {
		return errors.New("github org is nil")
	}
	if org.Deleted {
		return nil
	}

	version := org.Version
	req := &p42.DeleteGithubOrgRequest{
		OrgID:   org.OrgID,
		Version: version,
	}

	if err := h.client.DeleteGithubOrg(ctx, req); err != nil {
		return fmt.Errorf("delete github org: %w", err)
	}

	return nil
}
