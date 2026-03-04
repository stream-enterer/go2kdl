package kdl

import (
	"errors"
	"fmt"
	"strings"
)

// FormatError returns a multi-line, human-readable diagnostic string for err.
// If err is an *Error or Errors with source text, the output includes a
// numbered source line with a caret pointing at the error column:
//
//	error: unexpected } before node entry
//	  --> input.kdl:3:5
//	   |
//	 3 | foo { }
//	   |       ^
//
// If err is any other error, FormatError returns err.Error().
func FormatError(err error) string {
	return FormatErrorWithFilename(err, "")
}

// FormatErrorWithFilename is like FormatError but includes filename in the
// location line (e.g. " --> filename:3:5") when filename is non-empty.
func FormatErrorWithFilename(err error, filename string) string {
	if err == nil {
		return ""
	}

	var kdlErr *Error
	if errors.As(err, &kdlErr) {
		return formatSingle(kdlErr, filename)
	}

	var kdlErrs Errors
	if errors.As(err, &kdlErrs) {
		var b strings.Builder
		for i := range kdlErrs {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(formatSingle(&kdlErrs[i], filename))
		}
		return b.String()
	}

	return err.Error()
}

func formatSingle(e *Error, filename string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "error: %s\n", e.Message)

	if e.Span.Line <= 0 {
		return b.String()
	}

	if filename != "" {
		fmt.Fprintf(&b, " --> %s:%d:%d\n", filename, e.Span.Line, e.Span.Column)
	} else {
		fmt.Fprintf(&b, " --> %d:%d\n", e.Span.Line, e.Span.Column)
	}

	if len(e.Source) == 0 {
		return b.String()
	}

	// Extract the source line.
	line := extractSourceLine(e.Source, e.Span.Offset)
	lineNo := fmt.Sprintf("%d", e.Span.Line)
	gutter := strings.Repeat(" ", len(lineNo))

	caretLen := max(1, e.Span.Length)
	fmt.Fprintf(&b, " %s |\n", gutter)
	fmt.Fprintf(&b, " %s | %s\n", lineNo, line)
	fmt.Fprintf(&b, " %s | %s%s\n", gutter, strings.Repeat(" ", max(0, e.Span.Column-1)), strings.Repeat("^", caretLen))

	return b.String()
}

// extractSourceLine returns the text of the line containing offset.
func extractSourceLine(source []byte, offset int) string {
	if offset > len(source) {
		offset = len(source)
	}

	// Find start of line.
	start := offset
	for start > 0 && source[start-1] != '\n' {
		start--
	}

	// Find end of line.
	end := offset
	for end < len(source) && source[end] != '\n' && source[end] != '\r' {
		end++
	}

	s := string(source[start:end])
	// Replace tabs with spaces for consistent display.
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}
