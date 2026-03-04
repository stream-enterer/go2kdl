package kdl

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stream-enterer/go2kdl/document"
)

// TestFailCasesReturnStructuredError verifies that every _fail compliance
// test case returns a *Error with a populated Span.
func TestFailCasesReturnStructuredError(t *testing.T) {
	inputDir := filepath.Join(testCasesDir, "input")
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("failed to read input dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".kdl")
		if !strings.HasSuffix(name, "_fail") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join(inputDir, entry.Name())
			inputData, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("failed to read input file: %v", err)
			}

			_, parseErr := Parse(bytes.NewReader(inputData))
			if parseErr == nil {
				t.Fatalf("expected parse error for %s, but parsing succeeded", name)
			}

			var kdlErr *Error
			if !errors.As(parseErr, &kdlErr) {
				t.Errorf("expected *Error, got %T: %v", parseErr, parseErr)
				return
			}
			if kdlErr.Span.Line <= 0 {
				t.Errorf("expected Span.Line > 0, got %d for %s (message: %s)", kdlErr.Span.Line, name, kdlErr.Message)
			}
		})
	}
}

// TestStructuredErrorExactSpan parses a known-bad input and checks exact span values.
func TestStructuredErrorExactSpan(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLine   int
		wantCol    int
		wantSubstr string
	}{
		{
			name:       "unexpected token on line 1",
			input:      "}",
			wantLine:   1,
			wantCol:    1,
			wantSubstr: "unexpected",
		},
		{
			name:       "error on line 2",
			input:      "foo 1\n{",
			wantLine:   2,
			wantCol:    1,
			wantSubstr: "unexpected",
		},
		{
			name:       "reserved keyword",
			input:      "true",
			wantLine:   1,
			wantCol:    5, // scanner position is after consuming "true"
			wantSubstr: "reserved keyword",
		},
		{
			name:       "unterminated string",
			input:      "foo \"hello",
			wantLine:   1,
			wantCol:    11, // scanner position is at end of input
			wantSubstr: "unexpected EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(bytes.NewReader([]byte(tt.input)))
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var kdlErr *Error
			if !errors.As(err, &kdlErr) {
				t.Fatalf("expected *Error, got %T: %v", err, err)
			}

			if kdlErr.Span.Line != tt.wantLine {
				t.Errorf("Line = %d, want %d (message: %s)", kdlErr.Span.Line, tt.wantLine, kdlErr.Message)
			}
			if kdlErr.Span.Column != tt.wantCol {
				t.Errorf("Column = %d, want %d (message: %s)", kdlErr.Span.Column, tt.wantCol, kdlErr.Message)
			}
			if !strings.Contains(kdlErr.Message, tt.wantSubstr) {
				t.Errorf("Message = %q, want substring %q", kdlErr.Message, tt.wantSubstr)
			}
		})
	}
}

// TestFormatErrorOutput verifies the FormatError pretty-printer.
func TestFormatErrorOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantContains []string
	}{
		{
			name:  "unexpected brace",
			input: "foo\n  }\n",
			wantContains: []string{
				"error:",
				"-->",
				"|",
				"^",
			},
		},
		{
			name:  "reserved keyword",
			input: "true\n",
			wantContains: []string{
				"error:",
				"reserved keyword",
				"^",
			},
		},
		{
			name:  "unterminated string",
			input: "foo \"hello\n",
			wantContains: []string{
				"error:",
				"^",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(bytes.NewReader([]byte(tt.input)))
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			formatted := FormatError(err)
			for _, want := range tt.wantContains {
				if !strings.Contains(formatted, want) {
					t.Errorf("FormatError output missing %q:\n%s", want, formatted)
				}
			}
		})
	}
}

// TestFormatErrorFallback verifies FormatError works for non-kdl errors.
func TestFormatErrorFallback(t *testing.T) {
	err := errors.New("generic error")
	got := FormatError(err)
	if got != "generic error" {
		t.Errorf("FormatError for plain error = %q, want %q", got, "generic error")
	}
}

// TestNodeAndValueSpans verifies that parsed nodes and values have correct spans.
func TestNodeAndValueSpans(t *testing.T) {
	input := "first 1\nsecond \"hello\"\n"
	doc, err := Parse(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(doc.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(doc.Nodes))
	}

	// First node: "first" starts at line 1, column 1, offset 0
	n0 := doc.Nodes[0]
	if n0.Span.Line != 1 {
		t.Errorf("first node: Span.Line = %d, want 1", n0.Span.Line)
	}
	if n0.Span.Column != 1 {
		t.Errorf("first node: Span.Column = %d, want 1", n0.Span.Column)
	}
	if n0.Span.Offset != 0 {
		t.Errorf("first node: Span.Offset = %d, want 0", n0.Span.Offset)
	}
	if n0.Span.Length != 5 { // "first"
		t.Errorf("first node: Span.Length = %d, want 5", n0.Span.Length)
	}

	// First node argument "1" at offset 6
	if len(n0.Arguments) != 1 {
		t.Fatalf("first node: expected 1 argument, got %d", len(n0.Arguments))
	}
	a0 := n0.Arguments[0]
	if a0.Span.Line != 1 {
		t.Errorf("first arg: Span.Line = %d, want 1", a0.Span.Line)
	}
	if a0.Span.Offset != 6 {
		t.Errorf("first arg: Span.Offset = %d, want 6", a0.Span.Offset)
	}

	// Second node: "second" starts at line 2, column 1, offset 8
	n1 := doc.Nodes[1]
	if n1.Span.Line != 2 {
		t.Errorf("second node: Span.Line = %d, want 2", n1.Span.Line)
	}
	if n1.Span.Column != 1 {
		t.Errorf("second node: Span.Column = %d, want 1", n1.Span.Column)
	}
	if n1.Span.Offset != 8 {
		t.Errorf("second node: Span.Offset = %d, want 8", n1.Span.Offset)
	}

	// Second node argument "hello"
	if len(n1.Arguments) != 1 {
		t.Fatalf("second node: expected 1 argument, got %d", len(n1.Arguments))
	}
	a1 := n1.Arguments[0]
	if a1.Span.Line != 2 {
		t.Errorf("second arg: Span.Line = %d, want 2", a1.Span.Line)
	}
}

// TestProgrammaticNodeZeroSpan ensures programmatically built nodes have zero spans.
func TestProgrammaticNodeZeroSpan(t *testing.T) {
	n := document.NewNode()
	n.SetName("test")
	n.AddArgument("hello", "")

	zero := document.Span{}
	if n.Span != zero {
		t.Errorf("programmatic node span = %+v, want zero", n.Span)
	}
	if n.Name.Span != zero {
		t.Errorf("programmatic node name span = %+v, want zero", n.Name.Span)
	}
	if n.Arguments[0].Span != zero {
		t.Errorf("programmatic argument span = %+v, want zero", n.Arguments[0].Span)
	}
}

// TestErrorsType tests the Errors collection type.
func TestErrorsType(t *testing.T) {
	errs := Errors{
		{Message: "first error", Span: document.Span{Line: 1, Column: 1}},
		{Message: "second error", Span: document.Span{Line: 2, Column: 5}},
	}

	got := errs.Error()
	if !strings.Contains(got, "first error") || !strings.Contains(got, "second error") {
		t.Errorf("Errors.Error() = %q, expected both messages", got)
	}
}

// TestFormatErrorNil verifies that FormatError(nil) returns "".
func TestFormatErrorNil(t *testing.T) {
	if got := FormatError(nil); got != "" {
		t.Errorf("FormatError(nil) = %q, want empty string", got)
	}
}

// TestErrorsUnwrapAs verifies that errors.As can reach into an Errors collection.
func TestErrorsUnwrapAs(t *testing.T) {
	errs := Errors{
		{Message: "first", Span: document.Span{Line: 1, Column: 1}},
		{Message: "second", Span: document.Span{Line: 2, Column: 5}},
	}

	var kdlErr *Error
	if !errors.As(errs, &kdlErr) {
		t.Fatal("errors.As(Errors, *Error) returned false, want true")
	}
	if kdlErr.Message != "first" {
		t.Errorf("unwrapped error message = %q, want %q", kdlErr.Message, "first")
	}
}

// TestMultiCharCaret verifies that FormatError renders multi-char underlines.
func TestMultiCharCaret(t *testing.T) {
	e := &Error{
		Message: "bad token",
		Span:    document.Span{Line: 1, Column: 5, Offset: 4, Length: 4},
		Source:  []byte("foo bars baz"),
	}
	formatted := FormatError(e)
	if !strings.Contains(formatted, "^^^^") {
		t.Errorf("expected 4-char caret underline, got:\n%s", formatted)
	}
}

// TestFormatErrorWithFilename verifies the filename variant.
func TestFormatErrorWithFilename(t *testing.T) {
	e := &Error{
		Message: "oops",
		Span:    document.Span{Line: 3, Column: 5, Offset: 20},
		Source:  []byte("line1\nline2\nline3 some stuff here\n"),
	}
	formatted := FormatErrorWithFilename(e, "input.kdl")
	if !strings.Contains(formatted, "input.kdl:3:5") {
		t.Errorf("expected filename in location line, got:\n%s", formatted)
	}
}

// TestExtractSourceLineEdgeCases tests extractSourceLine with edge cases.
func TestExtractSourceLineEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		source string
		offset int
		want   string
	}{
		{"offset 0", "hello world", 0, "hello world"},
		{"past EOF", "hello", 100, "hello"},
		{"empty source", "", 0, ""},
		{"no newlines", "single line", 5, "single line"},
		{"at newline boundary", "abc\ndef", 4, "def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSourceLine([]byte(tt.source), tt.offset)
			if got != tt.want {
				t.Errorf("extractSourceLine(%q, %d) = %q, want %q", tt.source, tt.offset, got, tt.want)
			}
		})
	}
}

// TestErrorImplementsError verifies the error interface.
func TestErrorImplementsError(t *testing.T) {
	e := &Error{
		Message: "test message",
		Span:    document.Span{Line: 3, Column: 5},
	}

	var err error = e
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("Error.Error() = %q, want to contain 'line 3'", err.Error())
	}
	if !strings.Contains(err.Error(), "column 5") {
		t.Errorf("Error.Error() = %q, want to contain 'column 5'", err.Error())
	}
	if !strings.Contains(err.Error(), "test message") {
		t.Errorf("Error.Error() = %q, want to contain 'test message'", err.Error())
	}
}
