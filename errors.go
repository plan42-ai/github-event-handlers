package githubevents

import "errors"

// ErrUnknownEvent signals that an Event was passed to Handle whose EventType() does not
// match any registered handler.
var ErrUnknownEvent = errors.New("github-event-handlers: unknown event type")
