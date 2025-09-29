package client // or pkg/errors if you prefer

import (
	"fmt"
	"net/http"
)

// StatusError represents an unexpected HTTP response code
type StatusError struct {
	Code int
}

// Error implements the error interface
func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected status %d: %s", e.Code, http.StatusText(e.Code))
}

// New creates a new StatusError
func New(code int) error {
	return &StatusError{Code: code}
}
