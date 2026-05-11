package github

import (
	"net/http"

	"github.com/google/go-github/v81/github"
)

// ParseWebHook parses the raw webhook payload into a typed go-github event value.
// This wraps github.ParseWebHook so consumers do not need to import go-github directly.
func ParseWebHook(messageType string, payload []byte) (any, error) {
	return github.ParseWebHook(messageType, payload)
}

// WebHookType returns the X-GitHub-Event header value from the request.
// This wraps github.WebHookType so consumers do not need to import go-github directly.
func WebHookType(r *http.Request) string {
	return github.WebHookType(r)
}

// Ptr returns a pointer to the given value, equivalent to go-github's github.Ptr.
func Ptr[T any](v T) *T { return &v }
