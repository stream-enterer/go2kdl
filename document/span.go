package document

// Span identifies a range in source text.
// Zero value means "no span available".
type Span struct {
	// Offset is the byte offset of the start of the span in the input.
	Offset int
	// Length is the byte length of the span.
	Length int
	// Line is the 1-based line number of the start of the span.
	Line int
	// Column is the 1-based column (in runes, not bytes) of the start.
	Column int
}
