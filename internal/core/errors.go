package core

import (
	"context"
	"errors"
)

// Code classifies an error so the UI can choose the right presentation.
type Code string

const (
	CodeUsage         Code = "usage"
	CodeInput         Code = "input"
	CodeEngineMissing Code = "engine_missing"
	CodeEngineFailed  Code = "engine_failed"
	CodeOutput        Code = "output"
	CodeInternal      Code = "internal"
	CodeCanceled      Code = "canceled"
)

// Error is a user-facing failure with an actionable hint. The full cause stays
// available for logs; the UI shows Message and Hint.
type Error struct {
	Code    Code
	Op      string
	Path    string
	Message string
	Hint    string
	Cause   error
}

func NewError(code Code, op, path, message, hint string) *Error {
	return &Error{Code: code, Op: op, Path: path, Message: message, Hint: hint}
}

func Wrap(code Code, op, path string, cause error, hint string) *Error {
	e := &Error{Code: code, Op: op, Path: path, Hint: hint, Cause: cause}
	if cause != nil {
		e.Message = cause.Error()
	}
	return e
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if msg == "" && e.Cause != nil {
		msg = e.Cause.Error()
	}
	if msg == "" {
		msg = string(e.Code)
	}
	if e.Path != "" {
		msg = e.Path + ": " + msg
	}
	if e.Op != "" {
		msg = e.Op + ": " + msg
	}
	return msg
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CodeOf returns the error's code, or CodeInternal for unclassified errors.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return CodeCanceled
	}
	return CodeInternal
}

// HintOf returns the actionable hint attached to err, if any.
func HintOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Hint
	}
	return ""
}

// IsCanceled reports whether err represents user or shutdown cancellation.
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || CodeOf(err) == CodeCanceled
}
