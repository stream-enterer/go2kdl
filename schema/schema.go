package schema

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/stream-enterer/go2kdl/document"
)

// Schema is a compiled schema ready for validation.
// A Schema is safe for concurrent use by multiple goroutines.
type Schema struct {
	doc *schemaDoc
}

// Load parses a KDL schema from r and returns a compiled Schema.
func Load(r io.Reader) (*Schema, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading schema: %w", err)
	}
	return LoadBytes(b)
}

// LoadBytes is a convenience wrapper around Load.
func LoadBytes(b []byte) (*Schema, error) {
	doc, err := parseKDL(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}
	sd, err := compile(doc)
	if err != nil {
		return nil, err
	}
	return &Schema{doc: sd}, nil
}

// Validate checks doc against the schema and returns nil if valid.
// On failure, returns *ValidationResult (which implements error).
func (s *Schema) Validate(doc *document.Document) error {
	v := &validator{
		schema: s.doc,
	}
	v.validateDocument(doc)
	if len(v.errors) == 0 {
		return nil
	}
	sort.Slice(v.errors, func(i, j int) bool {
		return v.errors[i].Span.Offset < v.errors[j].Span.Offset
	})
	return &ValidationResult{Errors: v.errors}
}

// ValidationResult holds all violations from a single validation run.
type ValidationResult struct {
	Errors []ValidationError
}

func (r *ValidationResult) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "schema validation failed (%d error", len(r.Errors))
	if len(r.Errors) != 1 {
		sb.WriteByte('s')
	}
	sb.WriteString("):")
	for _, e := range r.Errors {
		sb.WriteString("\n  ")
		path := e.PathString()
		if path != "" {
			sb.WriteString(path)
			sb.WriteString(": ")
		}
		sb.WriteString(e.Message)
	}
	return sb.String()
}

// PathSegment identifies one element in the path to a violation.
type PathSegment struct {
	Name string
	Kind PathKind
}

// PathKind distinguishes what a path segment refers to.
type PathKind int

const (
	KindNode     PathKind = iota // a node
	KindProperty                 // a property on a node
	KindArgument                 // a positional argument on a node
)

// ValidationError is a single schema violation.
type ValidationError struct {
	Message  string
	Path     []PathSegment
	SchemaID string
	Span     document.Span
}

// PathString returns a human-readable path like "server > port[prop]".
func (e *ValidationError) PathString() string {
	if len(e.Path) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, seg := range e.Path {
		if i > 0 && seg.Kind == KindNode {
			sb.WriteString(" > ")
		}
		sb.WriteString(seg.Name)
		switch seg.Kind {
		case KindProperty:
			sb.WriteString("[prop]")
		case KindArgument:
			sb.WriteString("[arg]")
		}
	}
	return sb.String()
}
