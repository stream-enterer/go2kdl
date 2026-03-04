package tokenizer

import "fmt"

// ScanError is a structured scan error carrying source location.
type ScanError struct {
	// Message is the error description.
	Message string
	// Line is the 1-based line number.
	Line int
	// Column is the 1-based column number.
	Column int
	// Offset is the byte offset in the input.
	Offset int
	// Length is the byte length of the erroneous span (0 if unknown).
	Length int
	// Source is the full input buffer (may be nil for streaming).
	Source []byte
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("scan failed: %s at line %d, column %d", e.Message, e.Line, e.Column)
}
