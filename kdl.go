package kdl

import (
	"io"

	"github.com/stream-enterer/go2kdl/document"
	"github.com/stream-enterer/go2kdl/internal/generator"
	"github.com/stream-enterer/go2kdl/internal/parser"
	"github.com/stream-enterer/go2kdl/internal/tokenizer"
)

func parse(s *tokenizer.Scanner) (*document.Document, error) {
	defer s.Close()

	p := parser.New()
	opts := parser.ParseContextOptions{RelaxedNonCompliant: s.RelaxedNonCompliant}
	if s.ParseComments {
		opts.Flags |= parser.ParseComments
	}
	c := p.NewContextOptions(opts)
	c.SetSource(s.Raw())
	for s.Scan() {
		if err := p.Parse(c, s.Token()); err != nil {
			return nil, toError(err)
		}
	}
	if s.Err() != nil {
		return nil, toError(s.Err())
	}

	doc := c.Document()
	src := s.Raw()
	doc.Source = src
	if end := c.LastNodeEnd(); end < len(src) {
		doc.TrailingBytes = src[end:]
	}
	return doc, nil
}

// toError converts internal scanner/parser errors to *Error.
func toError(err error) error {
	if se, ok := err.(*tokenizer.ScanError); ok {
		return &Error{
			Message: se.Message,
			Span: document.Span{
				Offset: se.Offset,
				Length: se.Length,
				Line:   se.Line,
				Column: se.Column,
			},
			Source: se.Source,
		}
	}
	if pe, ok := err.(*parser.ParseError); ok {
		return &Error{
			Message: pe.Message,
			Span: document.Span{
				Offset: pe.Offset,
				Length: pe.Length,
				Line:   pe.Line,
				Column: pe.Column,
			},
			Source: pe.Source,
		}
	}
	return err
}

type ParseOptions = parser.ParseContextOptions

var DefaultParseOptions = parser.ParseContextOptions{}

// Parse parses a KDL document from r and returns the parsed Document, or a non-nil error on failure
func Parse(r io.Reader) (*document.Document, error) {
	return ParseWithOptions(r, DefaultParseOptions)
}

func ParseWithOptions(r io.Reader, opts ParseOptions) (*document.Document, error) {
	s := tokenizer.New(r)
	s.RelaxedNonCompliant = opts.RelaxedNonCompliant
	s.ParseComments = opts.Flags.Has(parser.ParseComments)
	return parse(s)
}

type GenerateOptions = generator.Options

var DefaultGenerateOptions = generator.DefaultOptions

// Autoformat parses a KDL document from r and writes a freshly formatted version to w.
func Autoformat(r io.Reader, w io.Writer) error {
	return AutoformatWithOptions(r, w, DefaultGenerateOptions)
}

// AutoformatWithOptions parses a KDL document from r and writes a freshly formatted version to w
// using the given options. PreserveFormatting is forced to false.
func AutoformatWithOptions(r io.Reader, w io.Writer, opts GenerateOptions) error {
	doc, err := Parse(r)
	if err != nil {
		return err
	}
	opts.PreserveFormatting = false
	return GenerateWithOptions(doc, w, opts)
}

// Generate writes to w a well-formatted KDL document generated from doc, or a non-nil error on failure
func Generate(doc *document.Document, w io.Writer) error {
	return GenerateWithOptions(doc, w, DefaultGenerateOptions)
}

// GenerateWithOptions writes to w a well-formatted KDL document generated from doc, or a non-nil error on failure
func GenerateWithOptions(doc *document.Document, w io.Writer, opts GenerateOptions) error {
	g := generator.NewOptions(w, opts)
	return g.Generate(doc)
}
