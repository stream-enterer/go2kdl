package document

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/stream-enterer/go2kdl/internal/tokenizer"
)

// ValueFlag represents flags for a Value
type ValueFlag uint8

const (
	// FlagNone indicates no flag is set
	FlagNone ValueFlag = iota
	// FlagRaw specifies that this Value should be output in RawString notation (#"foo\n"#)
	FlagRaw
	// FlagQuoted specifies that this Value should be output in FormattedString notation ("foo\\n")
	FlagQuoted
	// FlagBinary specifies that this Value should be output in binary notation (0b10101010)
	FlagBinary
	// FlagOctal specifies that this Value should be output in octal notation (0o751)
	FlagOctal
	// FlagHexadecimal specifies that this Value should be output in hexadecimal notation (0xdeadbeef)
	FlagHexadecimal
	// FlagSuffixed specifies that this value is a suffixed number
	FlagBareSuffixed
)

// Value represents a value in a KDL document
type Value struct {
	// Type is the value type, if an annotation was provided
	Type TypeAnnotation
	// Value is the actual value
	Value interface{}
	// Flag is any flag assigned for use in output
	Flag ValueFlag
	// HasDecimal indicates whether the original source representation included a decimal point.
	// This is used to preserve formatting for float values in E notation (e.g., 1.0E+10 vs 1E+10).
	HasDecimal bool
	// Span is the position of this value in source (zero if unavailable,
	// e.g. programmatically constructed values).
	Span Span
	// Raw holds the original source text for this value token (including type
	// annotation if present). When non-nil and the value has not been mutated,
	// the generator can emit these bytes verbatim.
	//
	// Known limitation: type annotation whitespace is normalized on re-emission
	// (e.g. "( u8 )" → "(u8)").
	Raw *RawSegment
}

// SetValue updates the value and nils the Raw segment to mark it as dirty.
func (v *Value) SetValue(val interface{}) { v.Value = val; v.Raw = nil }

// SetType updates the type annotation and nils the Raw segment to mark it as dirty.
func (v *Value) SetType(t TypeAnnotation) { v.Type = t; v.Raw = nil }

// SetFlag updates the formatting flag and nils the Raw segment to mark it as dirty.
func (v *Value) SetFlag(f ValueFlag) { v.Flag = f; v.Raw = nil }

// valueOpts specify options for rendering Values as strings
type valueOpts int

const (
	// if a numeric value was originally in octal, binary, or hex representation, output it the same way
	voUseNumericFlags valueOpts = 1 << iota
	// if a string was originally in raw, quoted, or bare representation, try to output it the same way with fallback to quoted
	voStrictStringFlags
	// strings are output bare if possible, otherwise quoted
	voSimpleString
	// force unquoted, bare output regardless of the string's original representation
	voNoQuotes
	// force quoted or raw representation of strings
	voNoBare
)

// AppendTo appends the simple string representation of this Value to b using decimal numbers, and returns the expanded
// buffer.
func (v *Value) AppendTo(b []byte) []byte {
	return v.value(b, voSimpleString)
}

// value appends the string representation of this Value to b using the specified opts, and returns the expanded buffer
func (v *Value) value(b []byte, opts valueOpts) []byte {
	haveOpt := func(opt valueOpts) bool {
		return (opts & opt) != 0
	}
	if v.Value == nil {
		return append(b, "#null"...)
	}

	base := 10
	prefix := ""
	if haveOpt(voUseNumericFlags) {
		switch v.Flag {
		case FlagBinary:
			base = 2
			prefix = "0b"
			if b == nil {
				b = make([]byte, 0, 10)
			}
		case FlagOctal:
			base = 8
			prefix = "0o"
			if b == nil {
				b = make([]byte, 0, 10)
			}
		case FlagHexadecimal:
			base = 16
			prefix = "0x"
			b = make([]byte, 0, 18)
		}
	}

	switch x := v.Value.(type) {
	case uint:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, uint64(x), base)
	case uint8:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, uint64(x), base)
	case uint16:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, uint64(x), base)
	case uint32:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, uint64(x), base)
	case uint64:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, x, base)
	case uintptr:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendUint(b, uint64(x), base)
	case int:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendInt(b, int64(x), base)
	case int8:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendInt(b, int64(x), base)
	case int16:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendInt(b, int64(x), base)
	case int32:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendInt(b, int64(x), base)
	case int64:
		if base != 10 {
			b = append(b, prefix...)
		}
		b = strconv.AppendInt(b, x, base)
	case float32:
		f64 := float64(x)
		if math.IsNaN(f64) {
			b = append(b, "#nan"...)
		} else if math.IsInf(f64, 1) {
			b = append(b, "#inf"...)
		} else if math.IsInf(f64, -1) {
			b = append(b, "#-inf"...)
		} else {
			l10 := math.Log10(math.Abs(f64))
			if !math.IsInf(l10, 0) && (l10 > 9 || l10 < -9) {
				b = strconv.AppendFloat(b, f64, 'E', -1, 32)
			} else {
				// make sure floats in decimal notation always include a decimal point
				b = strconv.AppendFloat(b, f64, 'f', -1, 32)
				if _, frac := math.Modf(f64); frac == 0.0 {
					b = append(b, '.', '0')
				}
			}
		}
	case float64:
		if math.IsNaN(x) {
			b = append(b, "#nan"...)
		} else if math.IsInf(x, 1) {
			b = append(b, "#inf"...)
		} else if math.IsInf(x, -1) {
			b = append(b, "#-inf"...)
		} else {
			l10 := math.Log10(math.Abs(x))
			if !math.IsInf(l10, 0) && (l10 > 9 || l10 < -9) {
				b = strconv.AppendFloat(b, x, 'E', -1, 64)
			} else {
				// make sure floats in decimal notation always include a decimal point
				b = strconv.AppendFloat(b, x, 'f', -1, 64)
				if _, frac := math.Modf(float64(x)); frac == 0.0 {
					b = append(b, '.', '0')
				}
			}
		}
	case bool:
		if x {
			b = append(b, "#true"...)
		} else {
			b = append(b, "#false"...)
		}
	case string:

		isBare := tokenizer.IsBareIdentifier(x, 0)

		if b == nil {
			size := len(x)
			if !isBare {
				size += 16
			}
			b = make([]byte, 0, size)
		}

		if v.Flag == FlagBareSuffixed || (!haveOpt(voNoBare) && (haveOpt(voNoQuotes) || (isBare && haveOpt(voSimpleString)))) {
			b = append(b, x...)
		} else {
			if v.Flag == FlagRaw && haveOpt(voStrictStringFlags) {
				b = AppendRawString(b, x)
			} else if v.Flag == FlagQuoted || (v.Flag == FlagRaw && !haveOpt(voStrictStringFlags)) {
				b = AppendQuotedString(b, x, '"')
			} else if isBare && !haveOpt(voNoBare) {
				b = append(b, x...)
			} else {
				b = AppendQuotedString(b, x, '"')
			}
		}

	case *big.Int:
		b = x.Append(b, base)
	case *big.Float:
		exp := x.MantExp(nil)
		if exp > 9 || exp < -9 {
			b = x.Append(b, 'E', -1)
		} else {
			b = x.Append(b, 'f', 6)
		}

	case SuffixedDecimal:
		b = append(b, x.Number...)
		b = append(b, x.Suffix...)

	default:
		formatted := fmt.Sprintf("%v", x)
		if haveOpt(voNoQuotes) {
			b = append(b, formatted...)
		} else {
			b = strconv.AppendQuote(b, formatted)
		}
	}
	return b
}

// string returns the KDL representation of this value with the specified opts, including type annotation if available,
// eg: (u8)1234
func (v *Value) string(opts valueOpts) string {
	var b []byte
	if len(v.Type) > 0 {
		b = make([]byte, 0, 32)
		b = append(b, '(')
		b = append(b, v.Type...)
		b = append(b, ')')
	}

	b = v.value(b, opts)
	return string(b)
}

// String returns the KDL representation of this Value, including type annotation, formatting numbers and strings per
// their Flags.
//
// This returns the exact input KDL (if any) that was used to generate this Value.
func (v *Value) String() string {
	return v.string(voStrictStringFlags | voUseNumericFlags)
}

// FormattedString is similar to String, but bare strings are converted to quoted strings.
//
// This is suitable for returning arguments and property values while preserving their original formatting.
func (v *Value) FormattedString() string {
	return v.string(voNoBare | voUseNumericFlags)
}

// UnformattedString is similar to String, but bare strings are converted to quoted strings and numbers are formatted
// in decimal notation.
//
// This is suitable for returning arguments and property values while ignoring their original formatting.
func (v *Value) UnformattedString() string {
	return v.string(voNoBare)
}

// NodeNameString returns the simplest possible KDL representation of this Value, including type annotation, formatting
// numbers in decimal notation and strings as bare strings if possible, otherwise quoted.
//
// This is suitable for returning a valid node name.
func (v *Value) NodeNameString() string {
	return v.string(voSimpleString)
}

// ValueString returns the unquoted, unescaped, un-type-hinted representation of this Value; numbers are formatted per
// their Flags, strings are always unquoted.
//
// This is suitable for passing as a []byte value to UnmarshalText.
func (v *Value) ValueString() string {
	b := make([]byte, 0, 32)
	return string(v.value(b, voNoQuotes|voUseNumericFlags))
}

// ResolvedValue returns the unquoted, unescaped, un-type-hinted Go representation of this value via an interface{}:
// - numbers are returned as the appropriate numeric type (int64, float64, *big.Int, *big.Float, etc),
// - bools are returned as a bool
// - nulls are returned as nil
// - strings are returned as strings containing the unquoted representation of the string
func (v *Value) ResolvedValue() interface{} {
	if _, ok := v.Value.(string); ok {
		return v.string(voNoQuotes)
	} else {
		return v.Value
	}
}

// isNonzeroSciNot returns true if b contains a string representation of a number in scientific notation with a nonzero
// coefficient.
func isNonzeroSciNot(b []byte) bool {
	coe, _, ok := bytes.Cut(b, []byte{'e'})
	if !ok {
		coe, _, ok = bytes.Cut(b, []byte{'E'})
	}
	if ok {
		coe = bytes.Trim(coe, "0")
		return len(coe) > 0 && !(len(coe) == 1 && coe[0] == '.')
	}
	return false
}

// parseNumber parses a number from b in the specified base, and returns an interface{} containing either a float64,
// an int64, a *big.Float, or a *big.Int, depending on the size and type of the number in b
func parseNumber(b []byte, base int) (interface{}, error) {
	if base != 10 {
		b = b[2:] // strip 0x, 0o, 0b
	}
	b = bytes.ReplaceAll(b, []byte{'_'}, []byte{})

	var (
		v   interface{}
		err error
	)
	float := bytes.IndexByte(b, '.') != -1 || (base == 10 && (bytes.IndexByte(b, 'e') != -1 || bytes.IndexByte(b, 'E') != -1))
	if float {
		if base != 10 {
			return nil, fmt.Errorf("parsing number %s: floating point numbers must be base 10 only", string(b))
		}

		var f float64
		f, err = strconv.ParseFloat(string(b), 64)

		// ParseFloat doesn't seem to generate ErrRange for tiny numbers in scientific notation (eg: 1.23E-1000); it
		// just returns 0, which is wrong. So if ParseFloat returns 0.0 and b contains a nonzero coefficient, we reparse
		// as a big.Float.
		if errors.Is(err, strconv.ErrRange) || (err == nil && f == 0.0 && isNonzeroSciNot(b)) {
			err = nil
			n := big.NewFloat(0)
			n.SetString(string(b))
			v = n
		} else {
			v = f
		}

	} else {
		v, err = strconv.ParseInt(string(b), base, 64)
		if errors.Is(err, strconv.ErrRange) {
			err = nil
			n := big.NewInt(0)
			n.SetString(string(b), base)
			v = n
		}
	}

	if err != nil {
		err = fmt.Errorf("parsing number %s: %w", string(b), err)
	}
	return v, err

}

// parseQuotedString parses a KDL FormattedString from b and returns the unquoted string, or a non-nil error on failure
func parseQuotedString(b []byte) (string, error) {
	v, err := UnquoteString(string(b))
	if err != nil {
		err = fmt.Errorf("parsing quoted string %s: %w", string(b), err)
	}
	return v, err
}

// parseRawString parses a KDLv2 RawString from b and returns the unquoted string, or a non-nil error on failure.
// KDLv2 raw strings use the syntax #"..."# (with N hashes on each side).
func parseRawString(b []byte) (string, error) {
	// the tokenizer has already validated the string format, so we can safely just use byte offsets
	// Format: ###"content"### where the number of # is the same on both sides
	p := bytes.IndexByte(b, '"')
	// p is the index of the first ", which equals the number of leading hashes
	// Opening delimiter is p+1 bytes (hashes + quote)
	// Closing delimiter is p+1 bytes (quote + hashes)
	b = b[p+1 : len(b)-(p+1)]
	return string(b), nil
}

// ValueFromToken creates and returns a Value representing the content of t, or a non-nil error on failure
func ValueFromToken(t tokenizer.Token) (*Value, error) {
	v := &Value{
		Span: Span{
			Offset: t.Offset,
			Length: len(t.Data),
			Line:   t.Line + 1,
			Column: t.Column + 1,
		},
	}
	var err error
	switch t.ID {
	case tokenizer.QuotedString:
		v.Value, err = parseQuotedString(t.Data)
		v.Flag = FlagQuoted
	case tokenizer.BareIdentifier:
		v.Value = string(t.Data)
	case tokenizer.Binary:
		v.Value, err = parseNumber(t.Data, 2)
		v.Flag = FlagBinary
	case tokenizer.RawString:
		v.Value, err = parseRawString(t.Data)
		v.Flag = FlagRaw
	case tokenizer.Decimal:
		v.Value, err = parseNumber(t.Data, 10)
		if bytes.IndexByte(t.Data, '.') != -1 {
			v.HasDecimal = true
		}
	case tokenizer.SuffixedDecimal:
		v.Value, err = ParseSuffixedDecimal(t.Data)
	case tokenizer.Octal:
		v.Value, err = parseNumber(t.Data, 8)
		v.Flag = FlagOctal
	case tokenizer.Hexadecimal:
		v.Value, err = parseNumber(t.Data, 16)
		v.Flag = FlagHexadecimal
	case tokenizer.MultilineString:
		v.Value, err = parseMultilineString(t.Data)
		v.Flag = FlagQuoted
	case tokenizer.MultilineRawString:
		v.Value, err = parseMultilineRawString(t.Data)
		v.Flag = FlagRaw
	case tokenizer.FloatKeyword:
		kw := string(t.Data)
		switch kw {
		case "#inf":
			v.Value = math.Inf(1)
		case "#-inf":
			v.Value = math.Inf(-1)
		case "#nan":
			v.Value = math.NaN()
		}
	case tokenizer.Boolean:
		// KDLv2: #true / #false
		v.Value = string(t.Data) == "#true"
	case tokenizer.Null:
		v.Value = nil
	}
	if err != nil {
		err = fmt.Errorf("value from token: %w", err)
	}

	// Preserve the raw token bytes for format-preserving output.
	if len(t.Data) > 0 {
		v.Raw = &RawSegment{Bytes: t.Data}
	}

	return v, err
}
