// internal/errors/errors.go
// Package errors provides centralized, reusable error types for the controller.
// Use these for consistent error handling across the application.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// -------------------- Sentinel Errors --------------------
// These are the base error types. Use errors.Is() to check against them.

var (
	// Authentication/Authorization
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Resource errors
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")

	// Input validation
	ErrBadRequest    = errors.New("bad request")
	ErrInvalidInput  = errors.New("invalid input")
	ErrMissingField  = errors.New("missing required field")
	ErrInvalidFormat = errors.New("invalid format")

	// Server errors
	ErrInternal       = errors.New("internal server error")
	ErrNotImplemented = errors.New("not implemented")
	ErrUnavailable    = errors.New("service unavailable")
)

// -------------------- HTTP Error --------------------
// HTTPError wraps an error with HTTP status code and optional details.

type HTTPError struct {
	Status  int    // HTTP status code
	Code    string // Optional error code for clients
	Message string // Human-readable message
	Cause   error  // Underlying error (not exposed to clients)
}

func (e *HTTPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *HTTPError) Unwrap() error {
	return e.Cause
}

// -------------------- HTTP Error Constructors --------------------

// BadRequest creates a 400 error.
func BadRequest(message string) *HTTPError {
	return &HTTPError{
		Status:  http.StatusBadRequest,
		Code:    "BAD_REQUEST",
		Message: message,
	}
}

// BadRequestf creates a 400 error with formatted message.
func BadRequestf(format string, args ...any) *HTTPError {
	return BadRequest(fmt.Sprintf(format, args...))
}

// Unauthorized creates a 401 error.
func Unauthorized(message string) *HTTPError {
	if message == "" {
		message = "unauthorized"
	}
	return &HTTPError{
		Status:  http.StatusUnauthorized,
		Code:    "UNAUTHORIZED",
		Message: message,
	}
}

// Forbidden creates a 403 error.
func Forbidden(message string) *HTTPError {
	if message == "" {
		message = "forbidden"
	}
	return &HTTPError{
		Status:  http.StatusForbidden,
		Code:    "FORBIDDEN",
		Message: message,
	}
}

// NotFound creates a 404 error.
func NotFound(resource string) *HTTPError {
	message := "not found"
	if resource != "" {
		message = resource + " not found"
	}
	return &HTTPError{
		Status:  http.StatusNotFound,
		Code:    "NOT_FOUND",
		Message: message,
	}
}

// Conflict creates a 409 error.
func Conflict(message string) *HTTPError {
	return &HTTPError{
		Status:  http.StatusConflict,
		Code:    "CONFLICT",
		Message: message,
	}
}

// InternalError creates a 500 error.
func InternalError(message string, cause error) *HTTPError {
	if message == "" {
		message = "internal server error"
	}
	return &HTTPError{
		Status:  http.StatusInternalServerError,
		Code:    "INTERNAL_ERROR",
		Message: message,
		Cause:   cause,
	}
}

// -------------------- Error Mapping --------------------

// StatusFromError returns the appropriate HTTP status code for an error.
// Falls back to 500 for unknown errors.
func StatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}

	// Check for HTTPError first
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status
	}

	// Check sentinel errors
	switch {
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAlreadyExists), errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrMissingField), errors.Is(err, ErrInvalidFormat):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotImplemented):
		return http.StatusNotImplemented
	case errors.Is(err, ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// WrapField wraps an error with field context.
func WrapField(field string, err error) error {
	return fmt.Errorf("%s: %w", field, err)
}

// FieldError creates a validation error for a specific field.
func FieldError(field, message string) error {
	return fmt.Errorf("%s: %s", field, message)
}
