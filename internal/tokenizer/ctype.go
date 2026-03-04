package tokenizer

import (
	"github.com/stream-enterer/go2kdl/relaxed"
)

// isWhiteSpace returns true if c is a whitespace character.
// In KDLv2, BOM (\uFEFF) is NOT whitespace - it is only allowed at document start.
func isWhiteSpace(c rune) bool {
	switch c {
	case // unicode-space
		'\t', ' ',
		'\u00A0',
		'\u1680',
		'\u2000',
		'\u2001',
		'\u2002',
		'\u2003',
		'\u2004',
		'\u2005',
		'\u2006',
		'\u2007',
		'\u2008',
		'\u2009',
		'\u200A',
		'\u202F',
		'\u205F',
		'\u3000':
		return true
	default:
		return false
	}
}

// isNewline returns true if c is a newline character.
// In KDLv2, vertical tab (\u000B) is also a newline character.
func isNewline(c rune) bool {
	switch c {
	case '\r', '\n', '\u000B', '\u0085', '\u000c', '\u2028', '\u2029':
		return true
	default:
		return false
	}
}

// isLineSpace returns true if c is a whitespace or newline character
func isLineSpace(c rune) bool {
	return isWhiteSpace(c) || isNewline(c)
}

// isDigit returns true if c is a digit
func isDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

// isSeparator returns true if c whitespace, a newline, or a semicolon
func isSeparator(c rune) bool {
	return isWhiteSpace(c) || isNewline(c) || c == ';'
}

// isBareIdentifierStartChar indicates whether c is a valid first character for a bare identifier. Note that this
// returns true if c is + or -, in which case the second character must not be a digit.
func isBareIdentifierStartChar(c rune, r relaxed.Flags) bool {
	if !isBareIdentifierChar(c, r) {
		return false
	}
	if isDigit(c) {
		return false
	}

	return true
}

// isBareIdentifierChar indicates whether c is a valid character for a bare identifier.
// In KDLv2, `<`, `>`, `,` are allowed but `#` is disallowed.
func isBareIdentifierChar(c rune, r relaxed.Flags) bool {
	if isLineSpace(c) {
		return false
	}
	if c <= 0x20 || c > 0x10FFFF {
		return false
	}
	if isDisallowedCodePoint(c) {
		return false
	}
	switch c {
	case '{', '}', '#', ';', '[', ']', '=':
		return false
	case '(', ')', '/', '\\', '"':
		return r.Permit(relaxed.NGINXSyntax)
	case ':':
		return !r.Permit(relaxed.YAMLTOMLAssignments)
	default:
		return true
	}
}

// IsBareIdentifier returns true if s contains a valid BareIdentifier (a string that requires no quoting in KDL)
func IsBareIdentifier(s string, rf relaxed.Flags) bool {
	if len(s) == 0 {
		return false
	}

	first := true
	for _, r := range s {
		if first {
			if !isBareIdentifierStartChar(r, rf) {
				return false
			}
			first = false
		} else {
			if !isBareIdentifierChar(r, rf) {
				return false
			}
		}
	}
	return true
}

// isDisallowedCodePoint returns true if c is a code point that is disallowed in KDLv2 documents
// (except in specific positions like BOM at document start).
func isDisallowedCodePoint(c rune) bool {
	switch {
	case c >= '\u0000' && c <= '\u0008': // control chars below tab
		return true
	case c >= '\u000E' && c <= '\u001F': // control chars
		return true
	case c == '\u007F': // DELETE
		return true
	case c == '\u200E' || c == '\u200F': // bidi controls (LRM, RLM)
		return true
	case c >= '\u202A' && c <= '\u202E': // bidi controls (LRE, RLE, PDF, LRO, RLO)
		return true
	case c >= '\u2066' && c <= '\u2069': // bidi controls (LRI, RLI, FSI, PDI)
		return true
	case c == '\uFEFF': // BOM (disallowed except at document start)
		return true
	default:
		return false
	}
}
