package parser

import "fmt"

// ParseError is a structured parse error carrying source location.
type ParseError struct {
	// Message is the error description.
	Message string
	// Line is the 1-based line number (from the token).
	Line int
	// Column is the 1-based column number (from the token).
	Column int
	// Offset is the byte offset of the token in the input.
	Offset int
	// Length is the byte length of the token that triggered the error.
	Length int
	// Source is the full input buffer (may be nil).
	Source []byte
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse failed: %s at line %d, column %d", e.Message, e.Line, e.Column)
}
