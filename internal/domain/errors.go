package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation   ErrorCode = "validation_failed"
	CodeInvalidState ErrorCode = "invalid_state"
	CodeNotFound     ErrorCode = "not_found"
	CodeConflict     ErrorCode = "revision_conflict"
	CodeIdempotency  ErrorCode = "idempotency_conflict"
	CodeCorrupt      ErrorCode = "storage_corrupt"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	Details any       `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func Validation(field, format string, args ...any) error {
	return &Error{Code: CodeValidation, Field: field, Message: fmt.Sprintf(format, args...)}
}

func ValidationDetails(field string, details any, format string, args ...any) error {
	return &Error{Code: CodeValidation, Field: field, Message: fmt.Sprintf(format, args...), Details: details}
}

func InvalidState(format string, args ...any) error {
	return &Error{Code: CodeInvalidState, Message: fmt.Sprintf(format, args...)}
}
