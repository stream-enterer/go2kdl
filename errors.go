package kdl

import (
	"fmt"
	"strings"

	"github.com/stream-enterer/go2kdl/document"
)

// Error is a structured parse error with source location and context.
type Error struct {
	// Message is the human-readable error description.
	Message string
	// Span locates the error in the source text. Zero value if unknown.
	Span document.Span
	// Source is the original input text (or nil if unavailable).
	// Retained so callers can render context snippets.
	Source []byte
}

// Error returns a compact diagnostic string suitable for logs.
func (e *Error) Error() string {
	if e.Span.Line > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Span.Line, e.Span.Column, e.Message)
	}
	return e.Message
}

// Errors collects one or more parse errors.
type Errors []Error

// Unwrap returns the individual errors so errors.As/errors.Is can
// traverse into the collection.
func (e Errors) Unwrap() []error {
	errs := make([]error, len(e))
	for i := range e {
		errs[i] = &e[i]
	}
	return errs
}

// Error joins messages, one per line.
func (e Errors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}
	var b strings.Builder
	for i := range e {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e[i].Error())
	}
	return b.String()
}
