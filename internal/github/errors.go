package github

import (
	"errors"
	"fmt"
)

// Sentinel errors for common GitHub API failure modes.
var (
	ErrRateLimited  = errors.New("github: rate limited")
	ErrNotFound     = errors.New("github: resource not found")
	ErrUnauthorized = errors.New("github: unauthorized")
	ErrConflict     = errors.New("github: conflict")
)

// APIError represents a GitHub API error response with status code and body.
type APIError struct {
	StatusCode int
	Body       string
	Err        error // underlying sentinel error, if any
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github API %d: %s", e.StatusCode, e.Body)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// newAPIError creates an APIError for the given HTTP status code and response body.
// It automatically maps well-known status codes to sentinel errors.
func newAPIError(statusCode int, body string) *APIError {
	var sentinel error
	switch statusCode {
	case 401:
		sentinel = ErrUnauthorized
	case 403:
		sentinel = ErrRateLimited
	case 404:
		sentinel = ErrNotFound
	case 409:
		sentinel = ErrConflict
	}
	return &APIError{
		StatusCode: statusCode,
		Body:       body,
		Err:        sentinel,
	}
}
