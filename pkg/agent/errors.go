package agent

import (
	"fmt"
	"strings"
)

// ErrConfigValidation indicates a configuration validation error
type ErrConfigValidation struct {
	Field   string
	Message string
}

func (e *ErrConfigValidation) Error() string {
	return fmt.Sprintf("config validation error: %s %s", e.Field, e.Message)
}

// ErrRegistrationFailed indicates machine registration failed
type ErrRegistrationFailed struct {
	Reason     string
	StatusCode int
}

func (e *ErrRegistrationFailed) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("registration failed (status %d): %s", e.StatusCode, e.Reason)
	}
	return fmt.Sprintf("registration failed: %s", e.Reason)
}

// From checks if an error is an ErrRegistrationFailed and populates fields
func (e *ErrRegistrationFailed) From(err error) bool {
	if err == nil {
		return false
	}
	if strings.HasPrefix(err.Error(), "registration failed") {
		e.Reason = err.Error()
		return true
	}
	return false
}

// ErrKeepaliveFailed indicates keepalive update failed
type ErrKeepaliveFailed struct {
	StatusCode int
	Body       string
}

func (e *ErrKeepaliveFailed) Error() string {
	return fmt.Sprintf("keepalive failed (status %d): %s", e.StatusCode, e.Body)
}

// From checks if an error is an ErrKeepaliveFailed and populates fields
func (e *ErrKeepaliveFailed) From(err error) bool {
	if err == nil {
		return false
	}
	if strings.HasPrefix(err.Error(), "keepalive failed") {
		e.Body = err.Error()
		return true
	}
	return false
}
