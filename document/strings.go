package document

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	// noEscapeTable maps each ASCII value to a boolean value indicating whether it does NOT require escapement
	noEscapeTable = [256]bool{}
	// hexTable maps each hexadecimal digit (0-9, a-f, and A-F) to its decimal value
	hexTable = [256]rune{}
)

func init() {
	// initialize the maps
	for i := 0; i <= 0x7e; i++ {
		noEscapeTable[i] = i >= 0x20 && i != '\\' && i != '"'
	}

	for r := '0'; r <= '9'; r++ {
		hexTable[r] = r - '0'
	}
	for r := 'a'; r <= 'f'; r++ {
		hexTable[r] = r - 'a' + 10
	}
	for r := 'A'; r <= 'F'; r++ {
		hexTable[r] = r - 'A' + 10
	}
}

// QuoteString returns s quoted for use as a KDL FormattedString
func QuoteString(s string) string {
	b := make([]byte, 0, len(s)*5/4)
	return string(AppendQuotedString(b, s, '"'))
}

// AppendQuotedString appends s, quoted for use as a KDL FormattedString, to b, and returns the expanded buffer.
//
// AppendQuotedString is based on the JSON string quoting function from the MIT-Licensed ZeroLog, Copyright (c) 2017
// Olivier Poitrey, but has been heavily modified to improve performance and use KDL string escapes instead of JSON.
func AppendQuotedString(b []byte, s string, quote byte) []byte {
	b = append(b, quote)

	// use uints for bounds-check elimination
	lenS := uint(len(s))
	// Loop through each character in the string.
	for i := uint(0); i < lenS; i++ {
		// Check if the character needs encoding. Control characters, slashes,
		// and the double quote need json encoding. Bytes above the ascii
		// boundary needs utf8 encoding.
		if !noEscapeTable[s[i]] {
			// We encountered a character that needs to be encoded. Switch
			// to complex version of the algorithm.

			start := uint(0)
			for i < lenS {
				c := s[i]
				if noEscapeTable[c] {
					i++
					continue
				}

				if c >= utf8.RuneSelf {
					r, size := utf8.DecodeRuneInString(s[i:])
					if r == utf8.RuneError && size == 1 {
						// In case of error, first append previous simple characters to
						// the byte slice if any and append a replacement character code
						// in place of the invalid sequence.
						if start < i {
							b = append(b, s[start:i]...)
						}
						b = append(b, `\ufffd`...)
						i += uint(size)
						start = i
						continue
					}
					i += uint(size)
					continue
				}

				// We encountered a character that needs to be encoded.
				// Let's append the previous simple characters to the byte slice
				// and switch our operation to read and encode the remainder
				// characters byte-by-byte.
				if start < i {
					b = append(b, s[start:i]...)
				}

				switch c {
				case quote, '\\':
					b = append(b, '\\', c)
				case '\n':
					b = append(b, '\\', 'n')
				case '\r':
					b = append(b, '\\', 'r')
				case '\t':
					b = append(b, '\\', 't')
				case '\b':
					b = append(b, '\\', 'b')
				case '\f':
					b = append(b, '\\', 'f')
				default:
					b = append(b, '\\', 'u')
					b = strconv.AppendUint(b, uint64(c), 16)
				}
				i++
				start = i
			}
			if start < lenS {
				b = append(b, s[start:]...)
			}

			b = append(b, quote)
			return b
		}
	}
	// The string has no need for encoding an therefore is directly
	// appended to the byte slice.
	b = append(b, s...)
	b = append(b, quote)

	return b
}

const empty = ""

// UnquoteString returns s unquoted from KDL FormattedString notation
func UnquoteString(s string) (string, error) {
	if len(s) == 0 {
		return empty, nil
	}
	q := s[0]
	switch q {
	case '"', '\'':
	default:
		return "", ErrInvalid
	}

	b := make([]byte, 0, len(s))
	b, err := AppendUnquotedString(b, s, q)
	return string(b), err
}

var ErrInvalid = errors.New("invalid quoted string")

// AppendUnquotedString appends s, unquoted from KDL FormattedString notation, to b and returns the expanded buffer.
//
// AppendUnquotedString was originally based on the JSON string quoting function from the MIT-Licensed ZeroLog,
// Copyright (c) 2017 Olivier Poitrey, but has been heavily modified to unquote KDL quoted strings.
func AppendUnquotedString(b []byte, s string, quote byte) ([]byte, error) {
	if len(s) < 2 || s[0] != quote || s[len(s)-1] != quote {
		return nil, ErrInvalid
	}
	// remove quotes
	s = s[1 : len(s)-1]

	// use uints for bounds-check elimination
	lenS := uint(len(s))
	// Loop through each character in the string.
	for i := uint(0); i < lenS; i++ {
		c := s[i]
		// Check if the character needs decoding.
		if c == '\\' || c >= utf8.RuneSelf {
			// We encountered a character that needs to be decoded. Switch
			// to complex version of the algorithm.

			start := uint(0)
			for i < lenS {
				c := s[i]
				if !(c == '\\' || c >= utf8.RuneSelf) {
					i++
					continue
				}

				if c >= utf8.RuneSelf {
					r, size := utf8.DecodeRuneInString(s[i:])
					if r == utf8.RuneError && size == 1 {
						// In case of error, first append previous simple characters to
						// the byte slice if any and append a replacement character code
						// in place of the invalid sequence.
						if start < i {
							b = append(b, s[start:i]...)
						}
						b = append(b, `\ufffd`...)
						i += uint(size)
						start = i
						continue
					}
					i += uint(size)
					continue
				}

				// We encountered a character that needs to be decoded.
				// Let's append the previous simple characters to the byte slice
				// and switch our operation to read and encode the remainder
				// characters byte-by-byte.
				if start < i {
					b = append(b, s[start:i]...)
				}

				i++
				if i == lenS {
					return b, ErrInvalid
				}
				c = s[i]

				// KDLv2: check if this is a whitespace escape (\ followed by whitespace/newline)
				// We need to handle multi-byte chars, so decode the rune at the current position
				isWsEscape := false
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x0B' || c == '\x0C' {
					isWsEscape = true
				} else if c >= utf8.RuneSelf {
					rn, _ := utf8.DecodeRuneInString(s[i:])
					if isUnquoteWhitespace(rn) {
						isWsEscape = true
					}
				}

				if isWsEscape {
					// KDLv2: whitespace escape - backslash followed by whitespace/newline
					// consumes ALL following whitespace (including newlines), producing no output
					for i < lenS {
						cb := s[i]
						if cb == ' ' || cb == '\t' || cb == '\n' || cb == '\r' || cb == '\x0B' || cb == '\x0C' {
							i++
						} else if cb >= utf8.RuneSelf {
							rn, sz := utf8.DecodeRuneInString(s[i:])
							if isUnquoteWhitespace(rn) {
								i += uint(sz)
							} else {
								break
							}
						} else {
							break
						}
					}
					start = i
					continue
				}

				switch c {
				case quote:
					b = append(b, quote)
				case '\\':
					b = append(b, '\\')
				case 'n':
					b = append(b, '\n')
				case 'r':
					b = append(b, '\r')
				case 't':
					b = append(b, '\t')
				case 'b':
					b = append(b, '\b')
				case 'f':
					b = append(b, '\f')
				case 's':
					// KDLv2: \s escapes to a single space (U+0020)
					b = append(b, ' ')
				case 'u':
					// make sure we have enough room for `{n}`
					if i+3 >= lenS || s[i+1] != '{' {
						return b, ErrInvalid
					}
					i += 2

					// find the closing `}`
					rstart := i
					for i < lenS && s[i] != '}' {
						i++
					}
					if i >= lenS {
						return b, ErrInvalid
					}
					if i-rstart > 6 {
						return b, ErrInvalid
					}

					// convert the hex digits, working backwards
					r := rune(0)
					factor := rune(1)
					for j := i - 1; j >= rstart; j-- {
						r += hexTable[s[j]] * factor
						factor *= 16
					}
					if r > 0x10FFFF {
						return b, ErrInvalid
					}
					// Reject Unicode surrogates (U+D800 through U+DFFF)
					if r >= 0xD800 && r <= 0xDFFF {
						return b, ErrInvalid
					}
					b = utf8.AppendRune(b, r)
				default:
					// KDLv2: reject invalid escape sequences
					return nil, fmt.Errorf("invalid escape sequence: \\%c", rune(c))
				}
				i++
				start = i
			}
			if start < lenS {
				b = append(b, s[start:]...)
			}

			return b, nil
		}
	}

	// The string has no need for decoding an therefore is directly
	// appended to the byte slice.
	b = append(b, s...)

	return b, nil
}

func rawString(s string) string {
	b := make([]byte, 0, 1+8*2+len(s))
	return string(AppendRawString(b, s))
}

// AppendRawString appends s, quoted for use as a KDLv2 RawString (#"..."#), to b and returns the expanded buffer.
func AppendRawString(b []byte, s string) []byte {
	// Determine the minimum number of '#' characters needed so that the closing
	// sequence ("# with N hashes) does not appear in the string content.
	// We need at least 1 hash.
	hashCount := 1
	for {
		// Build the closing sequence: " followed by hashCount #'s
		closing := make([]byte, 0, 1+hashCount)
		closing = append(closing, '"')
		for i := 0; i < hashCount; i++ {
			closing = append(closing, '#')
		}
		if !strings.Contains(s, string(closing)) {
			break
		}
		hashCount++
		if hashCount > 64 {
			// Fallback: should never happen in practice
			b = append(b, "#\"invalid\"#"...)
			return b
		}
	}

	minSpace := hashCount + 1 + len(s) + 1 + hashCount
	if cap(b)-len(b) < minSpace {
		n := make([]byte, 0, len(b)+minSpace)
		n = append(n, b...)
		b = n
	}

	// Opening: N hashes then "
	for i := 0; i < hashCount; i++ {
		b = append(b, '#')
	}
	b = append(b, '"')

	// Content
	b = append(b, s...)

	// Closing: " then N hashes
	b = append(b, '"')
	for i := 0; i < hashCount; i++ {
		b = append(b, '#')
	}
	return b
}

// isUnquoteWhitespace returns true if r is a whitespace or newline character
// for the purposes of whitespace escape handling in KDLv2 string unquoting.
func isUnquoteWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\x0B', '\x0C',
		'\u0085', '\u00A0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004',
		'\u2005', '\u2006', '\u2007', '\u2008', '\u2009',
		'\u200A', '\u202F', '\u205F', '\u3000',
		'\u2028', '\u2029':
		return true
	default:
		return false
	}
}

// isMultilineNewline returns true if the byte at s[i] starts a newline sequence,
// and returns the number of bytes consumed by that newline sequence.
// Recognized sequences: CR LF, CR, LF, VT (0x0B), FF (0x0C), NEL (U+0085),
// LS (U+2028), PS (U+2029).
func isMultilineNewline(s string, i int) (bool, int) {
	if i >= len(s) {
		return false, 0
	}
	c := s[i]
	switch c {
	case '\n':
		return true, 1
	case '\r':
		if i+1 < len(s) && s[i+1] == '\n' {
			return true, 2
		}
		return true, 1
	case '\x0B', '\x0C':
		return true, 1
	default:
		if c >= 0x80 {
			r, size := utf8.DecodeRuneInString(s[i:])
			switch r {
			case '\u0085', '\u2028', '\u2029':
				return true, size
			}
		}
	}
	return false, 0
}

// isMultilineWhitespace returns true if the rune r is a non-newline whitespace character
// for the purposes of multiline string prefix matching.
func isMultilineWhitespace(r rune) bool {
	switch r {
	case ' ', '\t',
		'\u00A0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004',
		'\u2005', '\u2006', '\u2007', '\u2008', '\u2009',
		'\u200A', '\u202F', '\u205F', '\u3000':
		return true
	default:
		return false
	}
}

// normalizeNewlines replaces all literal newline sequences in s with LF (\n).
// Handles CR LF, CR, LF, VT, FF, NEL (U+0085), LS (U+2028), PS (U+2029).
func normalizeNewlines(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if isNL, size := isMultilineNewline(s, i); isNL {
			b.WriteByte('\n')
			i += size
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// processWhitespaceEscapes resolves whitespace escapes (\ followed by whitespace/newline)
// in s, while leaving all other escape sequences (\\, \n, \t, etc.) untouched.
// A bare \ at end of string (not followed by anything) is left as-is for later
// escape processing to detect as an error.
func processWhitespaceEscapes(s string) string {
	// Fast path: no backslashes at all
	if !strings.ContainsRune(s, '\\') {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}

		// Found a backslash
		if i+1 >= len(s) {
			// Bare backslash at end - leave for later escape processing
			b.WriteByte('\\')
			i++
			continue
		}

		next := s[i+1]
		// Check if next char is another backslash: skip both (leave for later escape processing)
		if next == '\\' {
			b.WriteByte('\\')
			b.WriteByte('\\')
			i += 2
			continue
		}

		// Check if next char starts a whitespace/newline sequence
		isWs := false
		if next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == '\x0B' || next == '\x0C' {
			isWs = true
		} else if next >= 0x80 {
			r, _ := utf8.DecodeRuneInString(s[i+1:])
			isWs = isUnquoteWhitespace(r)
		}

		if isWs {
			// Whitespace escape: consume backslash and all following whitespace/newlines
			i++ // skip the backslash
			for i < len(s) {
				c := s[i]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x0B' || c == '\x0C' {
					i++
				} else if c >= 0x80 {
					r, sz := utf8.DecodeRuneInString(s[i:])
					if isUnquoteWhitespace(r) {
						i += sz
					} else {
						break
					}
				} else {
					break
				}
			}
			continue
		}

		// Not a whitespace escape - leave the backslash and following char for later processing
		b.WriteByte('\\')
		i++
	}
	return b.String()
}

// multilineStripAndDedent performs the common multiline string processing steps:
// strip triple-quote delimiters, strip first/last newline, normalize newlines,
// determine prefix, and dedent content lines.
//
// For quoted multiline strings, whitespace escapes must be processed before calling
// the dedent step. For raw multiline, no escape processing is needed.
//
// Parameters:
//   - s: the raw token string with delimiters already stripped and first newline removed
//   - prefix: the whitespace prefix determined from the closing line
//
// Returns the dedented content string or an error.
func multilineDedent(s string, prefix string) (string, error) {
	// Split into lines
	lines := strings.Split(s, "\n")

	// Process each line: strip prefix or handle whitespace-only lines
	for i, line := range lines {
		if len(line) == 0 {
			// Empty line - leave as empty
			continue
		}

		// Check if the line is whitespace-only (all characters are non-newline whitespace)
		wsOnly := true
		for _, r := range line {
			if !isMultilineWhitespace(r) {
				wsOnly = false
				break
			}
		}

		if wsOnly {
			// Whitespace-only lines become empty strings regardless of content
			lines[i] = ""
			continue
		}

		// Non-whitespace line: must start with the prefix
		if !strings.HasPrefix(line, prefix) {
			return "", fmt.Errorf("multiline string: line %d does not start with the expected whitespace prefix", i+1)
		}
		lines[i] = line[len(prefix):]
	}

	return strings.Join(lines, "\n"), nil
}

// processBackslashEscapes processes KDL backslash escape sequences in s (which has no surrounding
// quotes). This handles: \\, \n, \r, \t, \b, \f, \s, \", \u{XXXX}, and treats whitespace escapes
// as already processed (any remaining \ followed by whitespace is an error since it should have been
// handled in an earlier step). A bare \ at end of string is also an error.
func processBackslashEscapes(s string) (string, error) {
	// Fast path: no backslashes at all
	if !strings.ContainsRune(s, '\\') {
		return s, nil
	}

	b := make([]byte, 0, len(s))
	lenS := uint(len(s))
	start := uint(0)

	for i := uint(0); i < lenS; i++ {
		c := s[i]

		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				if start < i {
					b = append(b, s[start:i]...)
				}
				b = append(b, "\ufffd"...)
				i += uint(size)
				start = i
				continue
			}
			i += uint(size) - 1 // -1 because loop will i++
			continue
		}

		if c != '\\' {
			continue
		}

		// Found a backslash
		if start < i {
			b = append(b, s[start:i]...)
		}

		i++
		if i == lenS {
			return "", fmt.Errorf("unterminated escape sequence at end of string")
		}
		c = s[i]

		switch c {
		case '\\':
			b = append(b, '\\')
		case '"':
			b = append(b, '"')
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 's':
			b = append(b, ' ')
		case 'u':
			if i+3 >= lenS || s[i+1] != '{' {
				return "", ErrInvalid
			}
			i += 2

			rstart := i
			for i < lenS && s[i] != '}' {
				i++
			}
			if i >= lenS {
				return "", ErrInvalid
			}
			if i-rstart > 6 {
				return "", ErrInvalid
			}

			r := rune(0)
			factor := rune(1)
			for j := i - 1; j >= rstart; j-- {
				r += hexTable[s[j]] * factor
				factor *= 16
			}
			if r > 0x10FFFF {
				return "", ErrInvalid
			}
			// Reject Unicode surrogates (U+D800 through U+DFFF)
			if r >= 0xD800 && r <= 0xDFFF {
				return "", ErrInvalid
			}
			b = utf8.AppendRune(b, r)
		default:
			// Any other character after \ is invalid
			return "", fmt.Errorf("invalid escape sequence: \\%c", c)
		}
		i++ // move past the escape character
		start = i
		i-- // compensate for loop i++
	}

	if start < lenS {
		b = append(b, s[start:]...)
	}

	return string(b), nil
}

// parseMultilineString parses a KDLv2 multiline quoted string from b (including """ delimiters)
// and returns the processed string value.
//
// Processing order per KDLv2 spec:
// 1. Strip """ delimiters
// 2. Strip first newline after opening """
// 3. Normalize all literal newlines to LF
// 4. Process whitespace escapes on the entire content (including closing line)
// 5. Split on last newline to get content and closing line (prefix)
// 6. Validate closing line is whitespace-only = prefix
// 7. Dedent content lines by stripping the prefix
// 8. Process remaining escape sequences (\n, \t, \\, \u{...}, etc.)
func parseMultilineString(b []byte) (string, error) {
	s := string(b)

	// Step 1: Strip """ delimiters
	if len(s) < 6 || s[:3] != `"""` || s[len(s)-3:] != `"""` {
		return "", fmt.Errorf("multiline string: missing triple-quote delimiters")
	}
	s = s[3 : len(s)-3]

	// Step 2: Strip the first newline after opening """
	if len(s) == 0 {
		return "", fmt.Errorf("multiline string: opening \"\"\" must be followed by a newline")
	}
	if isNL, size := isMultilineNewline(s, 0); isNL {
		s = s[size:]
	} else {
		return "", fmt.Errorf("multiline string: opening \"\"\" must be followed by a newline")
	}

	// Step 3: Normalize all literal newlines to LF
	s = normalizeNewlines(s)

	// Step 4: Process whitespace escapes on the entire string (content + closing line).
	// This must happen before splitting so that whitespace escapes that span the boundary
	// between the last content line and the closing line are handled correctly.
	s = processWhitespaceEscapes(s)

	// Step 5: Find the last newline to split content from closing line
	lastNL := strings.LastIndex(s, "\n")
	if lastNL < 0 {
		// No newline found - the string is either empty or has only a closing line.
		// This happens with """\n""" or """\n\t""" (empty string with optional prefix).
		closingLine := s

		// Validate closing line is whitespace-only (it's the prefix)
		for _, r := range closingLine {
			if !isMultilineWhitespace(r) {
				return "", fmt.Errorf("multiline string: closing line contains non-whitespace characters")
			}
		}
		return "", nil
	}

	content := s[:lastNL]
	closingLine := s[lastNL+1:]

	// Step 6: Validate closing line is whitespace-only.
	for _, r := range closingLine {
		if !isMultilineWhitespace(r) {
			return "", fmt.Errorf("multiline string: closing line contains non-whitespace characters")
		}
	}
	prefix := closingLine

	// Step 7: Dedent content lines
	content, err := multilineDedent(content, prefix)
	if err != nil {
		return "", err
	}

	// Step 8: Process remaining escape sequences (without surrounding quotes)
	result, err := processBackslashEscapes(content)
	if err != nil {
		return "", fmt.Errorf("multiline string: %w", err)
	}

	return result, nil
}

// parseMultilineRawString parses a KDLv2 multiline raw string from b
// (including #"""..."""# delimiters with appropriate hash count)
// and returns the processed string value.
//
// Processing steps:
// 1. Strip #""" / """# delimiters (with hash count)
// 2. Strip first newline after opening delimiter
// 3. Normalize all literal newlines to LF
// 4. Determine prefix from closing line (must be whitespace-only)
// 5. Strip closing line
// 6. Dedent content lines by stripping the prefix
// NO escape processing (backslashes are literal)
func parseMultilineRawString(b []byte) (string, error) {
	s := string(b)

	// Determine hash count from the beginning
	hashCount := 0
	for i := 0; i < len(s) && s[i] == '#'; i++ {
		hashCount++
	}

	// Step 1: Strip #""" / """# delimiters
	openDelim := hashCount + 3  // hashes + """
	closeDelim := 3 + hashCount // """ + hashes

	if len(s) < openDelim+closeDelim {
		return "", fmt.Errorf("multiline raw string: token too short")
	}

	s = s[openDelim : len(s)-closeDelim]

	// Step 2: Strip the first newline after opening delimiter
	if len(s) == 0 {
		return "", fmt.Errorf("multiline raw string: opening delimiter must be followed by a newline")
	}
	if isNL, size := isMultilineNewline(s, 0); isNL {
		s = s[size:]
	} else {
		return "", fmt.Errorf("multiline raw string: opening delimiter must be followed by a newline")
	}

	// Step 3: Normalize all literal newlines to LF
	s = normalizeNewlines(s)

	// Step 4: Find the last newline to split content from closing line
	lastNL := strings.LastIndex(s, "\n")
	if lastNL < 0 {
		// No newline - empty string case (like #"""\n"""#)
		closingLine := s
		for _, r := range closingLine {
			if !isMultilineWhitespace(r) {
				return "", fmt.Errorf("multiline raw string: closing line contains non-whitespace characters")
			}
		}
		return "", nil
	}

	content := s[:lastNL]
	closingLine := s[lastNL+1:]

	// Validate closing line is whitespace-only
	for _, r := range closingLine {
		if !isMultilineWhitespace(r) {
			return "", fmt.Errorf("multiline raw string: closing line contains non-whitespace characters")
		}
	}
	prefix := closingLine

	// Step 5-6: Dedent content lines
	content, err := multilineDedent(content, prefix)
	if err != nil {
		return "", err
	}

	return content, nil
}
