package kdl

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stream-enterer/go2kdl/document"
	"github.com/stream-enterer/go2kdl/internal/tokenizer"
)

const testCasesDir = "testdata/test_cases"

func TestKDLv2Compliance(t *testing.T) {
	inputDir := filepath.Join(testCasesDir, "input")
	expectedDir := filepath.Join(testCasesDir, "expected_kdl")

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("failed to read input dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdl") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".kdl")
		isFail := strings.HasSuffix(name, "_fail")

		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join(inputDir, entry.Name())
			inputData, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("failed to read input file: %v", err)
			}

			doc, parseErr := Parse(bytes.NewReader(inputData))

			if isFail {
				if parseErr == nil {
					t.Errorf("expected parse error for %s, but parsing succeeded", name)
				}
				return
			}

			// Non-fail test: parsing should succeed
			if parseErr != nil {
				t.Fatalf("expected successful parse for %s, but got error: %v", name, parseErr)
			}

			// Check for expected output file
			expectedPath := filepath.Join(expectedDir, entry.Name())
			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Skipf("expected output file missing for %s, skipping", name)
				}
				t.Fatalf("failed to read expected file: %v", err)
			}

			// Generate output using our compliance formatter
			got := complianceFormatDocument(doc)
			want := string(expectedData)

			if got != want {
				t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s\n--- got (hex) ---\n%s\n--- want (hex) ---\n%s",
					name,
					got,
					want,
					complianceHexDump(got),
					complianceHexDump(want),
				)
			}
		})
	}
}

// complianceHexDump returns a compact hex representation useful for debugging whitespace differences.
func complianceHexDump(s string) string {
	var b strings.Builder
	for i, c := range []byte(s) {
		if i > 0 && i%16 == 0 {
			b.WriteByte('\n')
		} else if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02x", c)
	}
	return b.String()
}

// complianceFormatDocument produces the KDLv2 spec-compliant output for a parsed document.
// Empty documents produce a single newline.
func complianceFormatDocument(doc *document.Document) string {
	if len(doc.Nodes) == 0 {
		return "\n"
	}
	var b strings.Builder
	for _, node := range doc.Nodes {
		complianceFormatNode(&b, node, 0)
	}
	return b.String()
}

// complianceFormatNode writes a single node with its arguments, properties, and children.
func complianceFormatNode(b *strings.Builder, n *document.Node, depth int) {
	indent := strings.Repeat("    ", depth)

	b.WriteString(indent)

	// Type annotation
	if len(n.Type) > 0 {
		b.WriteByte('(')
		b.WriteString(complianceFormatIdentifier(complianceResolveTypeAnnotation(string(n.Type))))
		b.WriteByte(')')
	}

	// Node name: get the actual string value from the Value
	nodeName := n.Name.Value.(string)
	b.WriteString(complianceFormatIdentifier(nodeName))

	// Arguments
	for _, arg := range n.Arguments {
		b.WriteByte(' ')
		b.WriteString(complianceFormatValue(arg))
	}

	// Properties (alphabetical order, deduplicated - rightmost wins, which the parser handles)
	if n.Properties.Exist() {
		props := n.Properties.Unordered()
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := props[k]
			b.WriteByte(' ')
			b.WriteString(complianceFormatIdentifier(k))
			b.WriteByte('=')
			b.WriteString(complianceFormatValue(v))
		}
	}

	// Children
	if len(n.Children) > 0 {
		b.WriteString(" {\n")
		for _, child := range n.Children {
			complianceFormatNode(b, child, depth+1)
		}
		b.WriteString(indent)
		b.WriteByte('}')
	}

	b.WriteByte('\n')
}

// complianceResolveTypeAnnotation resolves a raw TypeAnnotation (which stores the raw token data)
// to its actual string value. The raw form could be:
//   - A bare identifier: "type" -> "type"
//   - A quoted string: "\"type/\"" -> "type/"
//   - A raw string: "#\"type\"#" -> "type"
func complianceResolveTypeAnnotation(raw string) string {
	if len(raw) == 0 {
		return raw
	}

	// Check if it's a quoted string
	if raw[0] == '"' {
		unquoted, err := document.UnquoteString(raw)
		if err == nil {
			return unquoted
		}
		// If unquoting fails, return as-is
		return raw
	}

	// Check if it's a raw string (starts with #)
	if raw[0] == '#' {
		// Raw string format: #"..."# or ##"..."## etc.
		hashCount := 0
		for i := 0; i < len(raw) && raw[i] == '#'; i++ {
			hashCount++
		}
		if hashCount < len(raw) && raw[hashCount] == '"' {
			// Extract content: skip opening hashes+quote, trim closing quote+hashes
			content := raw[hashCount+1 : len(raw)-(hashCount+1)]
			return content
		}
		return raw
	}

	// Bare identifier: return as-is
	return raw
}

// complianceFormatIdentifier returns a bare identifier if possible, or a quoted string otherwise.
func complianceFormatIdentifier(s string) string {
	if len(s) > 0 && tokenizer.IsBareIdentifier(s, 0) {
		return s
	}
	return complianceFormatQuotedString(s)
}

// complianceFormatValue returns the KDLv2 spec-compliant string representation of a value.
func complianceFormatValue(v *document.Value) string {
	var b strings.Builder

	// Type annotation
	if len(v.Type) > 0 {
		b.WriteByte('(')
		b.WriteString(complianceFormatIdentifier(complianceResolveTypeAnnotation(string(v.Type))))
		b.WriteByte(')')
	}

	b.WriteString(complianceFormatValueRaw(v))
	return b.String()
}

// complianceFormatValueRaw returns the raw value string (without type annotation).
func complianceFormatValueRaw(v *document.Value) string {
	if v.Value == nil {
		return "#null"
	}

	switch x := v.Value.(type) {
	case bool:
		if x {
			return "#true"
		}
		return "#false"

	case string:
		// Strings: use bare identifier if possible, quoted otherwise
		if len(x) > 0 && tokenizer.IsBareIdentifier(x, 0) {
			return x
		}
		return complianceFormatQuotedString(x)

	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)

	case float32:
		return complianceFormatFloat(float64(x), 32, v.HasDecimal)
	case float64:
		return complianceFormatFloat(x, 64, v.HasDecimal)

	case *big.Int:
		return x.String()
	case *big.Float:
		return complianceFormatBigFloat(x)

	default:
		return fmt.Sprintf("%v", x)
	}
}

// complianceFormatFloat formats a float64 value according to the KDLv2 spec test expectations.
func complianceFormatFloat(f float64, bitSize int, hasDecimal bool) string {
	if math.IsInf(f, 1) {
		return "#inf"
	}
	if math.IsInf(f, -1) {
		return "#-inf"
	}
	if math.IsNaN(f) {
		return "#nan"
	}

	// Use E notation for very large or very small numbers
	l10 := math.Log10(math.Abs(f))
	if !math.IsInf(l10, 0) && (l10 > 9 || l10 < -9) {
		s := strconv.FormatFloat(f, 'E', -1, bitSize)
		// If the original source had a decimal point, ensure it's preserved in E notation
		// (e.g., 1.0e10 -> 1.0E+10, but 1e10 -> 1E+10)
		if hasDecimal {
			if idx := strings.IndexByte(s, 'E'); idx >= 0 {
				mantissa := s[:idx]
				if !strings.Contains(mantissa, ".") {
					s = mantissa + ".0" + s[idx:]
				}
			}
		}
		return s
	}

	// Decimal notation
	s := strconv.FormatFloat(f, 'f', -1, bitSize)
	// Ensure decimal point is present
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// complianceFormatBigFloat formats a *big.Float value according to the KDLv2 spec test expectations.
func complianceFormatBigFloat(f *big.Float) string {
	// For big.Float, use E notation for large exponents
	exp := f.MantExp(nil)
	// Convert binary exponent to approximate base-10 exponent
	exp10 := int(float64(exp) * 0.30103) // log10(2) ~ 0.30103
	if exp10 > 9 || exp10 < -9 {
		return f.Text('E', -1)
	}
	s := f.Text('f', -1)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// complianceFormatQuotedString returns a KDLv2 quoted string representation.
// Uses named escapes where possible, \u{XX} for other control characters.
func complianceFormatQuotedString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7F {
				// Control character: use \u{XX} escape (KDLv2 format)
				fmt.Fprintf(&b, "\\u{%x}", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
