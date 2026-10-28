package providers

import (
	"errors"
	"fmt"
	"time"
)

type ErrorClass string

const (
	Auth      ErrorClass = "auth"
	Config    ErrorClass = "config"
	Quota     ErrorClass = "quota"
	Transient ErrorClass = "transient"
	Outage    ErrorClass = "outage"
)

// Error carries safe diagnostics only. msg must never contain response data.
type Error struct {
	Class        ErrorClass
	Code, Reason string
	Status       int
	RetryAt      time.Time
	msg          string
}

func NewError(class ErrorClass, code, reason string, status int, retryAt time.Time) *Error {
	return &Error{Class: class, Code: code, Reason: reason, Status: status, RetryAt: retryAt}
}
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	_ = e.msg // Deliberately exclude raw diagnostics from public error text.
	return fmt.Sprintf("provider %s %s (status %d)", e.Class, e.Code, e.Status)
}
func ClassOf(err error) ErrorClass {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ""
}

// FailureClass classifies delivery failures without coupling the dispatcher to providers.
func (e *Error) FailureClass() string {
	if e == nil {
		return "infra"
	}
	switch e.Code {
	case "E700", "E701", "E702":
		if (e.Class == Auth || e.Class == Config) && e.Status != 408 && e.Status != 409 && e.Status != 425 && e.Status/100 != 2 && e.Reason != "provider policy unreadable" {
			return "code"
		}
	}
	return "infra"
}
