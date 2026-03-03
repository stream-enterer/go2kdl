package tokenizer

import (
	"fmt"
	"io"

	"github.com/ar-go/go2kdl/relaxed"
)

// readWhitespace reads all whitespace starting from the current position. It does not return an error as in practice it
// is only called after r.peek() has already been invoked and returned a whitespace character, and thus at least one
// whitespace character will always be available.
func (s *Scanner) readWhitespace() []byte {
	ws, _ := s.readWhile(isWhiteSpace, 1)
	return ws
}

// skipWhitespace skips zero or more whitespace characters from the current position, and returns a non-nil error on
// failure
func (s *Scanner) skipWhitespace() error {
	_, err := s.readWhile(isWhiteSpace, 0)
	return err
}

// readMultiLineComment reads and returns a multiline comment from the current position, supporting nested /* and */
// sequences. It returns a non-nil error on failure.
func (s *Scanner) readMultiLineComment() ([]byte, error) {
	s.pushMark()
	defer s.popMark()

	depth := 0
	for {
		c, err := s.get()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}

		switch c {
		case '*':
			if next, err := s.peek(); err == nil && next == '/' {
				depth--
				s.skip()

				if depth == 0 {
					return s.copyFromMark(), nil
				}
			}

		case '/':
			if next, err := s.peek(); err == nil && next == '*' {
				depth++
				s.skip()
			}
		}
	}
}

// skipUntilNewline skips all characters from the current position until the next newline. It returns a non-nil error on
// failure.
func (s *Scanner) skipUntilNewline() error {
	escaped := false
	for {
		c, err := s.get()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch c {
		case '\\':
			escaped = true
			if err := s.skipWhitespace(); err != nil {
				return err
			}
		case '\r':
			// swallow error on peek, as it's still a valid newline if \r is not followed by \n
			if c, err := s.peek(); err == nil && c == '\n' {
				s.skip()
			}
			if escaped {
				escaped = false
			} else {
				return nil
			}

		case '\n', '\u000B', '\u0085', '\u000c', '\u2028', '\u2029':
			if escaped {
				escaped = false
			} else {
				return nil
			}
		default:
			escaped = false
		}
	}
}

// readSingleLineComment reads and returns a single-line comment from the current position, or a non-nil error on
// failure.
func (s *Scanner) readSingleLineComment() ([]byte, error) {
	literal, err := s.readUntil(isNewline, false)
	if err == io.ErrUnexpectedEOF {
		err = nil
	}
	return literal, err
}

// readRawString reads and returns a KDLv2 raw string from the input, or returns a non-nil error on failure.
// KDLv2 raw strings use the syntax #"..."# (with one or more # on each side).
// Also handles multiline raw strings: #"""..."""# (triple-quote variant).
// The caller has already peeked at the initial '#' but NOT consumed it.
// Returns the token ID (RawString or MultilineRawString), the raw bytes, and an error.
func (s *Scanner) readRawString(startHashes int) (TokenID, []byte, error) {
	s.pushMark()
	defer s.popMark()

	var (
		c   rune
		err error
	)

	// Consume the leading hashes
	for i := 0; i < startHashes; i++ {
		if c, err = s.get(); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Unknown, nil, err
		}
		if c != '#' {
			return Unknown, nil, fmt.Errorf("unexpected character %c", c)
		}
	}

	// Consume the opening quote
	if c, err = s.get(); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return Unknown, nil, err
	}
	if c != '"' {
		return Unknown, nil, fmt.Errorf("unexpected character %c", c)
	}

	// Check if this is a triple-quoted (multiline) raw string: #"""..."""#
	isMultiline := false
	if c2, err2 := s.peek(); err2 == nil && c2 == '"' {
		// Might be triple-quote - peek further
		if _, c3, err3 := s.peekTwo(); err3 == nil && c3 == '"' {
			// It's triple-quoted: consume the two remaining opening quotes
			s.skip() // second "
			s.skip() // third "
			isMultiline = true
		}
	}

	tokenID := RawString
	if isMultiline {
		tokenID = MultilineRawString
	}

	if isMultiline {
		// For multiline raw strings, we need to find """  followed by startHashes #'s
		consecutiveQuotes := 0
		for {
			if c, err = s.get(); err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return Unknown, nil, err
			}
			if c == '"' {
				consecutiveQuotes++
				if consecutiveQuotes >= 3 {
					// Check if followed by the right number of hashes
					matched := 0
					for matched < startHashes {
						if nc, nerr := s.peek(); nerr == nil && nc == '#' {
							s.skip()
							matched++
						} else {
							break
						}
					}
					if matched == startHashes {
						return tokenID, s.copyFromMark(), nil
					}
					// Not enough hashes - the quotes and partial hashes are content, keep going
					consecutiveQuotes = 0
				}
			} else {
				consecutiveQuotes = 0
			}
		}
	}

	// Single-line raw string: find closing " followed by startHashes #'s
	// KDLv2: literal newlines are NOT allowed in single-line raw strings (use triple-quoted multiline raw strings)
	for {
		if c, err = s.get(); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Unknown, nil, err
		}
		if c == '"' {
			// Check if followed by the right number of hashes
			matched := 0
			for matched < startHashes {
				if nc, nerr := s.peek(); nerr == nil && nc == '#' {
					s.skip()
					matched++
				} else {
					break
				}
			}
			if matched == startHashes {
				return tokenID, s.copyFromMark(), nil
			}
			// Not the closing sequence - the quote and partial hashes are content
		}
		if isNewline(c) {
			return Unknown, nil, fmt.Errorf("literal newlines are not allowed in single-line raw strings; use a multiline raw string (triple-quoted) instead")
		}
	}
}

// readQuotedString reads and returns a quoted string from the current position.
// It also handles KDLv2 triple-quoted multiline strings (""" ... """).
// Returns the token ID (QuotedString or MultilineString), the raw bytes, and an error.
func (s *Scanner) readQuotedString() (TokenID, []byte, error) {
	s.pushMark()
	defer s.popMark()

	var (
		c   rune
		err error
	)

	// Consume the opening quote
	if c, err = s.get(); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return Unknown, nil, err
	}
	if c != '"' {
		return Unknown, nil, fmt.Errorf("unexpected character %c", c)
	}

	// Check for triple-quote (""") by peeking at next two chars
	c2, err2 := s.peek()
	if err2 == nil && c2 == '"' {
		// Peek at the char after the second quote
		_, c3, err3 := s.peekTwo()
		if err3 == nil && c3 == '"' {
			// It's a triple-quoted multiline string: """
			s.skip() // consume second "
			s.skip() // consume third "

			// Read the multiline string content until closing """
			// Escape sequences are still processed (by the parser), but we
			// return the raw bytes including the triple-quote delimiters.
			escaped := false
			consecutiveQuotes := 0
			for {
				if c, err = s.get(); err != nil {
					if err == io.EOF {
						err = io.ErrUnexpectedEOF
					}
					return Unknown, nil, err
				}
				if escaped {
					escaped = false
					consecutiveQuotes = 0
					continue
				}
				if c == '\\' {
					escaped = true
					consecutiveQuotes = 0
					continue
				}
				if c == '"' {
					consecutiveQuotes++
					if consecutiveQuotes >= 3 {
						return MultilineString, s.copyFromMark(), nil
					}
				} else {
					consecutiveQuotes = 0
				}
			}
		}
		// Second char is " but third is not " -> this is an empty string ""
		// Fall through: the second " will be consumed by the regular logic below
	}

	// Regular single-line quoted string: read until unescaped closing "
	// KDLv2: literal (unescaped) newlines are NOT allowed in single-line strings.
	// However, \<whitespace> (whitespace escape) can span multiple lines.
	escaped := false
	for {
		if c, err = s.get(); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Unknown, nil, err
		}
		if escaped {
			if isNewline(c) || isWhiteSpace(c) {
				// Whitespace escape: \<ws> consumes all following whitespace including newlines.
				// Keep consuming whitespace/newlines until we find a non-whitespace character.
				for {
					nc, nerr := s.peek()
					if nerr != nil {
						break
					}
					if isNewline(nc) || isWhiteSpace(nc) {
						s.skip()
					} else {
						break
					}
				}
			}
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			return QuotedString, s.copyFromMark(), nil
		}
		if isNewline(c) {
			return Unknown, nil, fmt.Errorf("literal newlines are not allowed in single-line strings; use a multiline string (triple-quoted) instead")
		}
	}
}

func (s *Scanner) readSingleQuotedString() ([]byte, error) {
	return s.readQuotedStringQ('\'')
}

// readQuotedString reads and returns a quoted string from the current position, or returns a non-nil error on failure.
func (s *Scanner) readQuotedStringQ(q rune) ([]byte, error) {
	var (
		c   rune
		err error
	)
	if c, err = s.peek(); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if c != q {
		return nil, fmt.Errorf("unexpected character %c", c)
	}

	escaped := false
	done := false
	first := true
	return s.readWhile(func(c rune) bool {
		if first {
			// skip "
			first = false
			return true
		}
		if done {
			return false
		}
		switch c {
		case '\\':
			escaped = !escaped
		case q:
			if escaped {
				escaped = false
			} else {
				done = true
			}
		default:
			if escaped {
				escaped = false
			}
		}
		return true
	}, 2)

}

// readBareIdentifier reads a bare identifier from the current position and returns a TokenID representing its type.
// In KDLv2, `true`, `false`, `null` are just bare identifiers (string values), not special tokens.
// The keywords #true, #false, #null, #inf, #-inf, #nan are handled separately in scanner.go.
func (s *Scanner) readBareIdentifier() (TokenID, []byte, error) {
	var (
		c   rune
		err error
	)

	if c, err = s.peek(); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}

		return Unknown, nil, err
	}

	switch c {
	case '+', '-':
		_, c2, err2 := s.peekTwo()
		if err2 != nil {
			// Only one character (+ or - at EOF) - valid bare identifier
			break
		}
		if isDigit(c2) {
			return Unknown, nil, fmt.Errorf("unexpected character %c", c2)
		}
	default:
		if !isBareIdentifierStartChar(c, s.RelaxedNonCompliant) {
			return Unknown, nil, fmt.Errorf("unexpected character %c", c)
		}
		// In KDLv2, '.' followed by digit at start is invalid (e.g. .1)
		if c == '.' {
			if _, c2, err2 := s.peekTwo(); err2 == nil && isDigit(c2) {
				return Unknown, nil, fmt.Errorf("unexpected character %c", c2)
			}
		}
	}

	var literal []byte

	isBareIdentifierCharClosure := func(c rune) bool {
		return isBareIdentifierChar(c, s.RelaxedNonCompliant)
	}

	if literal, err = s.readWhile(isBareIdentifierCharClosure, 1); err != nil {
		return Unknown, nil, err
	}

	// In KDLv2, these words are reserved and cannot be used as bare identifiers.
	// They must be quoted (e.g., "true") to use as string values, or prefixed with #
	// (e.g., #true) for their keyword meaning.
	if !s.RelaxedNonCompliant.Permit(relaxed.NGINXSyntax) {
		switch string(literal) {
		case "true", "false", "null", "inf", "-inf", "nan":
			return Unknown, nil, fmt.Errorf("reserved keyword %q cannot be used as a bare identifier", string(literal))
		}
	}

	return BareIdentifier, literal, nil
}

// readIdentifier reads an identifier from the current position and returns a TokenID representing the identifier's
// type, a byte sequence representing the identifier, and a non-nil error on failure.
// In KDLv2, raw strings (r"...") are no longer valid - they use #"..."# syntax handled in scanner.go.
func (s *Scanner) readIdentifier() (TokenID, []byte, error) {
	c, err := s.peek()
	if err != nil {
		return Unknown, nil, err
	}

	if c <= 0x20 || c > 0x10FFFF {
		return Unknown, nil, fmt.Errorf("unexpected character %c", c)
	}

	switch c {
	case '"':
		s.log("quoted string, reading")
		tokenID, literal, err := s.readQuotedString()
		return tokenID, literal, err

	case '{', '}', '#', ';', '[', ']', '=':
		return Unknown, nil, fmt.Errorf("unexpected character %c", c)

	case '\\', '(', ')', '/':
		if !s.RelaxedNonCompliant.Permit(relaxed.NGINXSyntax) {
			return Unknown, nil, fmt.Errorf("unexpected character %c", c)
		}

	case '\'':
		if s.RelaxedNonCompliant.Permit(relaxed.NGINXSyntax) {
			s.log("single quoted string, reading")
			literal, err := s.readSingleQuotedString()
			return QuotedString, literal, err
		}
	}

	_, c2, err := s.peekTwo()
	if err == nil && !isBareIdentifierStartChar(c, s.RelaxedNonCompliant) && !(c == '-' && !isDigit(c2)) {
		s.log("not a valid bare identifier")
		return Unknown, nil, fmt.Errorf("unexpected character %c", c)
	}

	s.log("bare identifier, reading")
	tokenType, literal, err := s.readBareIdentifier()
	if err != nil {
		return tokenType, literal, err
	}

	// KDLv2: reject legacy raw string syntax r"..." and r#"..."#
	if !s.RelaxedNonCompliant.Permit(relaxed.NGINXSyntax) && string(literal) == "r" {
		if nc, nerr := s.peek(); nerr == nil && (nc == '"' || nc == '#') {
			return Unknown, nil, fmt.Errorf("legacy raw string syntax r\"...\" is not valid in KDLv2; use #\"...\"# instead")
		}
	}

	return tokenType, literal, err
}

// readInteger reads and returns an integer from the current position, or a non-nil error on failure
func (s *Scanner) readInteger() (TokenID, []byte, error) {
	tokenID := Decimal

	first := true
	validRune := func(c rune) bool {
		if first {
			first = false
			return isDigit(c) // cannot start with _
		}
		return isDigit(c) || c == '_'
	}

	hasMultiplier := false
	if s.RelaxedNonCompliant.Permit(relaxed.MultiplierSuffixes) {
		multiplierOK := true
		validRune = func(c rune) bool {
			if first {
				first = false
				if c == '+' || c == '-' {
					multiplierOK = false
					return true
				}
			}

			if multiplierOK {
				switch c {
				case 'h', 'm', 's', 'u', 'µ', 'k', 'K', 'M', 'g', 'G', 't', 'T', 'b':
					hasMultiplier = true
					return true
				}
			}

			return isDigit(c) || c == '_'
		}
	}
	data, err := s.readWhile(validRune, 1)

	if hasMultiplier {
		tokenID = SuffixedDecimal
	}

	return tokenID, data, err
}

// readSignedInteger reads and returns a signed integer from the current position, or a non-nil error on failure
func (s *Scanner) readSignedInteger() (TokenID, []byte, error) {
	s.pushMark()
	defer s.popMark()

	c, err := s.peek()
	if err != nil {
		return Unknown, nil, err
	}

	if c == '+' || c == '-' {
		s.skip()
	}

	tokenID, _, err := s.readInteger()
	return tokenID, s.copyFromMark(), err
}

// readDecimal reads and returns a a decimal value (either an integer or a floating point number) from the current
// position, or a non-nil error on failure
func (s *Scanner) readDecimal() (TokenID, []byte, error) {
	s.pushMark()
	defer s.popMark()

	tokenID, _, err := s.readSignedInteger()
	if err != nil {
		s.log("reading decimal: failed", "error", err)
		return tokenID, nil, err
	}

	// ignore any error at this point because we've already successfully read the initial signed integer
	// r.log("reading decimal: peeky")
	if c, err := s.peek(); err == nil {
		if c == '.' {
			s.skip()

			// r.log("reading decimal: unsigned integer")
			if tokenID, _, err = s.readInteger(); err != nil {
				s.log("reading decimal: failed", "error", err)
				return tokenID, nil, err
			}
		}

		// again, ignore any error
		if c, err := s.peek(); err == nil {
			if c == 'e' || c == 'E' {
				s.skip()
				// r.log("reading decimal: signed integer")
				if tokenID, _, err := s.readSignedInteger(); err != nil {
					s.log("reading decimal: failed", "error", err)
					return tokenID, nil, err
				}
			}
		}
	}

	if c, err := s.peek(); err == nil && !isSeparator(c) {
		if s.RelaxedNonCompliant.Permit(relaxed.NGINXSyntax) && isBareIdentifierChar(c, s.RelaxedNonCompliant) {
			// it's not actually a numeric identifier; parse as a bare string

			isBareIdentifierCharClosure := func(c rune) bool {
				return isBareIdentifierChar(c, s.RelaxedNonCompliant)
			}

			if _, err = s.readWhile(isBareIdentifierCharClosure, 1); err != nil {
				return Unknown, nil, err
			}

			tokenID = BareIdentifier
		} else {
			return tokenID, nil, fmt.Errorf("unexpected character %c", c)
		}
	}

	return tokenID, s.copyFromMark(), nil
}

// readNumericBase reads and returns a binary, octal, or hexadecimal number from the current position, ensuring that it
// is at least 3 characters in length (eg: 0xN), followed by whitespace or a newline, and that all characters are valid;
// returns a non-nil error on failure
func (s *Scanner) readNumericBase(valid func(c rune) bool) ([]byte, error) {
	lit, err := s.readWhile(valid, 3)
	if err == nil && lit[2] == '_' {
		// disallow 0x_
		return nil, fmt.Errorf("unexpected character _")
	}
	if err == nil {
		if c, err := s.peek(); err == nil && !isWhiteSpace(c) && !isNewline(c) {
			return nil, fmt.Errorf("unexpected character %c", c)
		}
	}
	return lit, err
}

// readHexadecimal reads and returns a hexadecimal number from the current position, or a non-nil error on failure
func (s *Scanner) readHexadecimal() ([]byte, error) {
	n := 0
	return s.readNumericBase(func(c rune) bool {
		if n < 2 {
			// skip 0x
			n++
			return true
		}
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '_'
	})
}

// readOctal reads and returns an octal number from the current position, or a non-nil error on failure
func (s *Scanner) readOctal() ([]byte, error) {
	n := 0
	return s.readNumericBase(func(c rune) bool {
		if n < 2 {
			// skip 0o
			n++
			return true
		}
		return (c >= '0' && c <= '7') || c == '_'
	})
}

// readBinary reads and returns a binary number from the current position, or a non-nil error on failure
func (s *Scanner) readBinary() ([]byte, error) {
	n := 0
	return s.readNumericBase(func(c rune) bool {
		if n < 2 {
			// skip 0b
			n++
			return true
		}
		return c == '0' || c == '1' || c == '_'
	})
}
