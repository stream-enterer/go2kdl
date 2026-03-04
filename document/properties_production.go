//go:build kdlunordered

//
// properties_unordered.go provides a simple map-based Properties type.
// This is the opt-in variant; enable with -tags kdlunordered.

package document

import (
	"sort"

	"github.com/stream-enterer/go2kdl/internal/tokenizer"
)

// Properties represents a list of properties for a Node
type Properties map[string]*Value

// Allocated indicates whether the property list has been allocated
func (p Properties) Allocated() bool {
	return p != nil
}

// Alloc allocates the property list
func (p *Properties) Alloc() {
	*p = make(map[string]*Value)
}

// Get returns Properties[key]
func (p Properties) Get(key string) (*Value, bool) {
	v, ok := p[key]
	return v, ok
}

// Len returns the number of properties
func (p Properties) Len() int {
	return len(p)
}

// Unordered returns the unordered property map; this simply passes through p in this implementation but is provided
// as it is necessary in the deterministic version
func (p Properties) Unordered() map[string]*Value {
	return p
}

// Add adds a property to the list
func (p Properties) Add(name string, val *Value) {
	p[name] = val
}

// Exist indicates whether any properties exist
func (p Properties) Exist() bool {
	return len(p) > 0
}

// Delete removes the property with the given name and returns true,
// or returns false if no such property exists.
func (p Properties) Delete(name string) bool {
	if _, exists := p[name]; !exists {
		return false
	}
	delete(p, name)
	return true
}

// sortedKeys returns the keys of the property map in sorted order for deterministic output
func (p Properties) sortedKeys() []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String returns the KDL representation of the property list, formatting numbers per their flags
func (p Properties) String() string {
	b := make([]byte, 0, len(p)*(1+8+1+8))
	for _, k := range p.sortedKeys() {
		v := p[k]
		b = append(b, ' ')
		if len(k) > 0 && tokenizer.IsBareIdentifier(k, 0) {
			b = append(b, k...)
		} else {
			b = AppendQuotedString(b, k, '"')
		}
		b = append(b, '=')
		// property values must always be quoted
		b = append(b, v.FormattedString()...)
	}
	return string(b)
}

// UnformattedString returns the KDL representation of the property list, formatting numbers in decimal
func (p Properties) UnformattedString() string {
	b := make([]byte, 0, len(p)*(1+8+1+8))
	for _, k := range p.sortedKeys() {
		v := p[k]
		b = append(b, ' ')
		if len(k) > 0 && tokenizer.IsBareIdentifier(k, 0) {
			b = append(b, k...)
		} else {
			b = AppendQuotedString(b, k, '"')
		}
		b = append(b, '=')
		// property values must always be quoted
		b = append(b, v.UnformattedString()...)
	}
	return string(b)
}

// AppendTo appends the KDL representation of the property list to b, formatting numbers in decimal, and returns b
func (p Properties) AppendTo(b []byte) []byte {
	required := len(p) * (1 + 8 + 1 + 8)
	if cap(b)-len(b) < required {
		r := make([]byte, 0, len(b)+required)
		r = append(r, b...)
		b = r
	}
	for _, k := range p.sortedKeys() {
		v := p[k]
		b = append(b, ' ')
		if len(k) > 0 && tokenizer.IsBareIdentifier(k, 0) {
			b = append(b, k...)
		} else {
			b = AppendQuotedString(b, k, '"')
		}
		b = append(b, '=')
		// property values must always be quoted
		b = append(b, v.UnformattedString()...)
	}
	return b
}
