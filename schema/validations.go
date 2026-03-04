package schema

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/stream-enterer/go2kdl/document"
)

// validation is the interface for all validation rules.
type validation interface {
	validate(v *document.Value) error
}

// typeValidation checks the KDL type annotation.
type typeValidation struct {
	typeName string
}

func (t *typeValidation) validate(v *document.Value) error {
	if string(v.Type) != t.typeName {
		if v.Type == "" {
			return fmt.Errorf("expected type (%s), got no type annotation", t.typeName)
		}
		return fmt.Errorf("expected type (%s), got (%s)", t.typeName, v.Type)
	}
	return nil
}

// enumValidation checks value against an explicit set.
type enumValidation struct {
	allowed []any
}

func (e *enumValidation) validate(v *document.Value) error {
	for _, a := range e.allowed {
		if valuesEqual(v.Value, a) {
			return nil
		}
	}
	return fmt.Errorf("value %v is not in allowed set %v", v.Value, e.allowed)
}

// patternValidation checks string values against a regex.
type patternValidation struct {
	pattern *regexp.Regexp
}

func (p *patternValidation) validate(v *document.Value) error {
	s, err := toString(v.Value)
	if err != nil {
		return fmt.Errorf("pattern validation requires a string value, got %T", v.Value)
	}
	if !p.pattern.MatchString(s) {
		return fmt.Errorf("value %q does not match pattern %s", s, p.pattern.String())
	}
	return nil
}

// stringLengthValidation checks min-length / max-length.
type stringLengthValidation struct {
	min *int
	max *int
}

func (sl *stringLengthValidation) validate(v *document.Value) error {
	s, err := toString(v.Value)
	if err != nil {
		return fmt.Errorf("string length validation requires a string value, got %T", v.Value)
	}
	n := len(s)
	if sl.min != nil && n < *sl.min {
		return fmt.Errorf("string length %d is less than minimum %d", n, *sl.min)
	}
	if sl.max != nil && n > *sl.max {
		return fmt.Errorf("string length %d is greater than maximum %d", n, *sl.max)
	}
	return nil
}

// formatValidation checks a declared format.
type formatValidation struct {
	format string
}

func (f *formatValidation) validate(v *document.Value) error {
	s, err := toString(v.Value)
	if err != nil {
		return fmt.Errorf("format validation requires a string value, got %T", v.Value)
	}
	checker, ok := formatCheckers[f.format]
	if !ok {
		// Unknown formats pass unconditionally (forward-compatible).
		return nil
	}
	return checker(s)
}

// numberRangeValidation checks >, >=, <, <=.
type numberRangeValidation struct {
	op    string
	value float64
}

func (nr *numberRangeValidation) validate(v *document.Value) error {
	f, err := toFloat64(v.Value)
	if err != nil {
		return fmt.Errorf("numeric range validation requires a numeric value, got %T", v.Value)
	}
	var pass bool
	switch nr.op {
	case ">":
		pass = f > nr.value
	case ">=":
		pass = f >= nr.value
	case "<":
		pass = f < nr.value
	case "<=":
		pass = f <= nr.value
	}
	if !pass {
		return fmt.Errorf("value %v does not satisfy %s %v", v.Value, nr.op, nr.value)
	}
	return nil
}

// numberMultipleValidation checks % (divisibility).
type numberMultipleValidation struct {
	divisor float64
}

func (nm *numberMultipleValidation) validate(v *document.Value) error {
	f, err := toFloat64(v.Value)
	if err != nil {
		return fmt.Errorf("numeric multiple validation requires a numeric value, got %T", v.Value)
	}
	rem := math.Mod(f, nm.divisor)
	if math.Abs(rem) > 1e-9 {
		return fmt.Errorf("value %v is not a multiple of %v", v.Value, nm.divisor)
	}
	return nil
}

// toFloat64 coerces a value to float64.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case *big.Int:
		f, _ := new(big.Float).SetInt(n).Float64()
		return f, nil
	case *big.Float:
		f, _ := n.Float64()
		return f, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// toString coerces a value to string.
func toString(v any) (string, error) {
	switch s := v.(type) {
	case string:
		return s, nil
	case document.SuffixedDecimal:
		return s.String(), nil
	default:
		return "", fmt.Errorf("cannot convert %T to string", v)
	}
}

// valuesEqual performs coerced comparison for enum matching.
func valuesEqual(a, b any) bool {
	// Try direct equality first.
	if a == b {
		return true
	}
	// Try numeric coercion.
	af, aErr := toFloat64(a)
	bf, bErr := toFloat64(b)
	if aErr == nil && bErr == nil {
		return af == bf
	}
	// Try string coercion.
	as, asErr := toString(a)
	bs, bsErr := toString(b)
	if asErr == nil && bsErr == nil {
		return as == bs
	}
	return false
}

// Format checkers.
var formatCheckers = map[string]func(string) error{
	"date-time": func(s string) error {
		_, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("value %q is not a valid date-time (RFC3339)", s)
		}
		return nil
	},
	"date": func(s string) error {
		_, err := time.Parse("2006-01-02", s)
		if err != nil {
			return fmt.Errorf("value %q is not a valid date", s)
		}
		return nil
	},
	"time": func(s string) error {
		_, err := time.Parse("15:04:05", s)
		if err != nil {
			return fmt.Errorf("value %q is not a valid time", s)
		}
		return nil
	},
	"email": func(s string) error {
		if !emailRegex.MatchString(s) {
			return fmt.Errorf("value %q is not a valid email", s)
		}
		return nil
	},
	"ipv4": func(s string) error {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil || strings.Contains(s, ":") {
			return fmt.Errorf("value %q is not a valid IPv4 address", s)
		}
		return nil
	},
	"ipv6": func(s string) error {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("value %q is not a valid IPv6 address", s)
		}
		return nil
	},
	"uuid": func(s string) error {
		if !uuidRegex.MatchString(s) {
			return fmt.Errorf("value %q is not a valid UUID", s)
		}
		return nil
	},
	"base64": func(s string) error {
		_, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("value %q is not valid base64", s)
		}
		return nil
	},
}

var (
	emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	uuidRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)
