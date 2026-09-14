// Package exitcode defines cover100's process exit codes and the error type
// that carries one.
//
// The vocabulary is deliberately small and stable: scripts branch on these
// numbers, so a new meaning never reuses an existing number.
package exitcode

import "fmt"

// Standard exit codes.
const (
	// Success means the report was written. Collection warnings do not change
	// this: a run that salvaged a partial Go profile still produced a report.
	Success = 0
	// InvalidArgs means a flag value or argument was missing or malformed.
	InvalidArgs = 2
	// NotFound means the scan path does not exist, is not a directory, or
	// contains no Go module and no Node package.
	NotFound = 3
	// Unexpected is the catch-all for runtime failures: an unwritable report
	// path, a port that could not be bound, a panicking dependency.
	Unexpected = 10
)

// Error carries a process exit code alongside a human-readable message. It
// satisfies both the error interface and the ExitCode() convention the
// top-level runner checks.
type Error struct {
	code  int
	msg   string
	cause error
}

func (e *Error) Error() string { return e.msg }

// ExitCode returns the process exit code for this error.
func (e *Error) ExitCode() int { return e.code }

// Unwrap preserves the underlying cause for errors.Is/errors.As.
func (e *Error) Unwrap() error { return e.cause }

// New creates an Error with the given exit code and message.
func New(code int, msg string) *Error { return &Error{code: code, msg: msg} }

// Newf creates an Error with the given exit code and formatted message.
func Newf(code int, format string, args ...any) *Error {
	return &Error{code: code, msg: fmt.Sprintf(format, args...)}
}

// Wrap maps cause to code without discarding its identity.
func Wrap(code int, msg string, cause error) *Error {
	return &Error{code: code, msg: msg, cause: cause}
}

// InvalidArgsError returns an exit-code-2 error.
func InvalidArgsError(msg string) *Error { return New(InvalidArgs, msg) }

// InvalidArgsErrorf returns an exit-code-2 error with a formatted message.
func InvalidArgsErrorf(format string, args ...any) *Error { return Newf(InvalidArgs, format, args...) }

// NotFoundError returns an exit-code-3 error.
func NotFoundError(msg string) *Error { return New(NotFound, msg) }

// NotFoundErrorf returns an exit-code-3 error with a formatted message.
func NotFoundErrorf(format string, args ...any) *Error { return Newf(NotFound, format, args...) }

// NotFoundErrorCause maps a lookup or scan failure to exit 3 while keeping the
// underlying cause wrapped for errors.Is/errors.As.
func NotFoundErrorCause(msg string, cause error) *Error { return Wrap(NotFound, msg, cause) }

// UnexpectedError returns an exit-code-10 error.
func UnexpectedError(msg string) *Error { return New(Unexpected, msg) }

// UnexpectedErrorf returns an exit-code-10 error with a formatted message.
func UnexpectedErrorf(format string, args ...any) *Error { return Newf(Unexpected, format, args...) }

// UnexpectedErrorCause maps a runtime cause to exit 10, keeping it wrapped.
func UnexpectedErrorCause(msg string, cause error) *Error {
	return Wrap(Unexpected, msg, cause)
}
