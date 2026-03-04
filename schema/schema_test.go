package schema

import (
	"errors"
	"strings"
	"testing"

	kdl "github.com/stream-enterer/go2kdl"
	"github.com/stream-enterer/go2kdl/document"
)

func mustParse(t *testing.T, input string) *document.Document {
	t.Helper()
	doc, err := kdl.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("failed to parse KDL: %v", err)
	}
	return doc
}

func mustLoad(t *testing.T, input string) *Schema {
	t.Helper()
	sch, err := LoadBytes([]byte(input))
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}
	return sch
}

// Test 1: Load valid schema
func TestLoadValidSchema(t *testing.T) {
	schema := `
document {
	info {
		title "Test Schema"
		description "A test schema"
		author "Test Author"
	}
	node "server" {
		prop "host" {
			required #true
			type "string"
		}
		value {
			type "u16"
		}
		children {
			node "endpoint" {
				value {
					type "string"
				}
			}
		}
	}
	tag "important" {
	}
	definitions {
		node "shared" id="shared-node" {
			prop "name" {
				required #true
			}
		}
	}
	node-names {
		pattern "^[a-z-]+$"
	}
	other-nodes-allowed #false
	other-tags-allowed #false
}
`
	sch := mustLoad(t, schema)
	if sch.doc == nil {
		t.Fatal("expected non-nil schema doc")
	}
	if sch.doc.info == nil {
		t.Fatal("expected info to be parsed")
	}
	if sch.doc.info.title != "Test Schema" {
		t.Errorf("expected title 'Test Schema', got %q", sch.doc.info.title)
	}
	if len(sch.doc.nodes) != 1 {
		t.Errorf("expected 1 node def, got %d", len(sch.doc.nodes))
	}
	if sch.doc.defs == nil {
		t.Fatal("expected definitions to be parsed")
	}
	if sch.doc.nodeNames == nil {
		t.Fatal("expected node-names to be parsed")
	}
}

// Test 2: Load invalid schema
func TestLoadInvalidSchema(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "missing document root",
			input: `node "server" {}`,
			want:  "only 'document' is allowed",
		},
		{
			name: "extra top-level node",
			input: `
document {}
extra {}
`,
			want: "only 'document' is allowed",
		},
		{
			name: "invalid pattern regex",
			input: `
document {
	node "test" {
		value {
			pattern "[invalid"
		}
	}
}
`,
			want: "invalid pattern",
		},
		{
			name: "broken ref",
			input: `
document {
	node "test" ref="[id=\"nonexistent\"]" {
	}
	definitions {
	}
}
`,
			want: "no matching node definition found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadBytes([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

// Test 3: Validate passing document
func TestValidatePassingDocument(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		prop "host" {
			required #true
		}
		value {
			type "u16"
		}
	}
}
`)
	doc := mustParse(t, `server "host"="localhost" (u16)8080`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 4: Validate unknown node
func TestValidateUnknownNode(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {}
	other-nodes-allowed #false
}
`)
	doc := mustParse(t, `
server
database
`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' error, got: %v", err)
	}
}

// Test 5: Validate missing required property
func TestValidateMissingRequiredProperty(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		prop "host" {
			required #true
		}
	}
}
`)
	doc := mustParse(t, `server`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "required but missing") {
		t.Errorf("expected 'required but missing' error, got: %v", err)
	}
}

// Test 6: Validate cardinality
func TestValidateCardinality(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		min 1
		max 1
	}
}
`)

	// Too few
	doc := mustParse(t, ``)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error for min cardinality")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("expected 'at least 1' error, got: %v", err)
	}

	// Too many
	doc = mustParse(t, `
server
server
`)
	err = sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error for max cardinality")
	}
	if !strings.Contains(err.Error(), "at most 1") {
		t.Errorf("expected 'at most 1' error, got: %v", err)
	}
}

// Test 7: Validate type annotation
func TestValidateTypeAnnotation(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "port" {
		value {
			type "u16"
		}
	}
}
`)
	// Wrong type
	doc := mustParse(t, `port (i32)8080`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected type (u16)") {
		t.Errorf("expected type error, got: %v", err)
	}

	// No type annotation
	doc = mustParse(t, `port 8080`)
	err = sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no type annotation") {
		t.Errorf("expected 'no type annotation' error, got: %v", err)
	}
}

// Test 8: Validate enum
func TestValidateEnum(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "log-level" {
		value {
			enum "debug" "info" "warn" "error"
		}
	}
}
`)
	// Valid
	doc := mustParse(t, `log-level "info"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid
	doc = mustParse(t, `log-level "trace"`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not in allowed set") {
		t.Errorf("expected enum error, got: %v", err)
	}
}

// Test 9: Validate pattern
func TestValidatePattern(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "name" {
		value {
			pattern "^[a-z]+$"
		}
	}
}
`)
	// Valid
	doc := mustParse(t, `name "hello"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid
	doc = mustParse(t, `name "Hello"`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not match pattern") {
		t.Errorf("expected pattern error, got: %v", err)
	}
}

// Test 10: Validate string length
func TestValidateStringLength(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "name" {
		value {
			min-length 1
			max-length 255
		}
	}
}
`)
	// Empty string (too short)
	doc := mustParse(t, `name ""`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "less than minimum") {
		t.Errorf("expected string length error, got: %v", err)
	}

	// Valid
	doc = mustParse(t, `name "hello"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 11: Validate numeric range
func TestValidateNumericRange(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "port" {
		value {
			">=" 0
			"<=" 65535
		}
	}
}
`)
	// Too low
	doc := mustParse(t, `port -1`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not satisfy") {
		t.Errorf("expected range error, got: %v", err)
	}

	// Valid
	doc = mustParse(t, `port 8080`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 12: Validate numeric multiple
func TestValidateNumericMultiple(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "count" {
		value {
			% 2
		}
	}
}
`)
	// Not a multiple
	doc := mustParse(t, `count 3`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a multiple") {
		t.Errorf("expected multiple error, got: %v", err)
	}

	// Valid
	doc = mustParse(t, `count 4`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 13: Validate format
func TestValidateFormat(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "ip" {
		value {
			format "ipv4"
		}
	}
}
`)
	// Invalid
	doc := mustParse(t, `ip "not-an-ip"`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a valid IPv4") {
		t.Errorf("expected format error, got: %v", err)
	}

	// Valid
	doc = mustParse(t, `ip "192.168.1.1"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 14: Validate children not allowed
func TestValidateChildrenNotAllowed(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "leaf" {
	}
}
`)
	doc := mustParse(t, `
leaf {
	child
}
`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not allow children") {
		t.Errorf("expected children error, got: %v", err)
	}
}

// Test 15: Validate wildcard node
func TestValidateWildcardNode(t *testing.T) {
	sch := mustLoad(t, `
document {
	node {
		value {
			type "string"
		}
	}
}
`)
	// Any node name should match the wildcard.
	doc := mustParse(t, `
anything (string)"hello"
something (string)"world"
`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 16: Validate wildcard prop
func TestValidateWildcardProp(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "config" {
		prop {
			type "string"
		}
		other-props-allowed #true
	}
}
`)
	// All props should be validated by the wildcard.
	doc := mustParse(t, `config "foo"=(string)"bar" "baz"=(string)"qux"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Wrong type on a prop.
	doc = mustParse(t, `config "foo"=(i32)42`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected type (string)") {
		t.Errorf("expected type error, got: %v", err)
	}
}

// Test 17: Validate ref resolution (ID selector)
func TestValidateRefResolution(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" ref="[id=\"port-def\"]" {
	}
	definitions {
		node "server" id="port-def" {
			prop "port" {
				required #true
			}
		}
	}
}
`)
	// Missing required prop inherited from ref.
	doc := mustParse(t, `server`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "required but missing") {
		t.Errorf("expected required error, got: %v", err)
	}

	// With the required prop.
	doc = mustParse(t, `server "port"=8080`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 18: Validate other-props-allowed
func TestValidateOtherPropsAllowed(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		prop "host" {}
		other-props-allowed #false
	}
}
`)
	doc := mustParse(t, `server "host"="localhost" "port"=8080`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' error, got: %v", err)
	}
}

// Test 19: Validate tag
func TestValidateTag(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		other-props-allowed #true
		tag "primary" {
			node "server" {
				prop "priority" {
					required #true
				}
			}
		}
	}
}
`)
	// Node with tag but missing required prop from tag def.
	doc := mustParse(t, `(primary)server`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "required but missing") {
		t.Errorf("expected tag validation error, got: %v", err)
	}

	// With required prop.
	doc = mustParse(t, `(primary)server "priority"=1`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 20: Multiple errors
func TestMultipleErrors(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		prop "host" {
			required #true
		}
		prop "port" {
			required #true
		}
	}
	other-nodes-allowed #false
}
`)
	doc := mustParse(t, `
server
unknown
`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	var vr *ValidationResult
	if !errors.As(err, &vr) {
		t.Fatal("expected *ValidationResult")
	}
	// Should have errors for: missing host, missing port, unknown node.
	if len(vr.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(vr.Errors), err)
	}
}

// Test 21: Value count
func TestValueCount(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "point" {
		value {
			min 2
			max 4
		}
	}
}
`)
	// Too few
	doc := mustParse(t, `point 1`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("expected value count error, got: %v", err)
	}

	// Valid
	doc = mustParse(t, `point 1 2 3`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 22: Wildcard + named precedence
func TestWildcardAndNamedPrecedence(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {
		prop "host" {
			required #true
		}
		other-props-allowed #true
	}
	node {
		value {
			min 1
		}
		other-props-allowed #true
	}
}
`)
	// "server" matches both named and wildcard.
	// Named requires "host" prop, wildcard requires at least 1 arg.
	doc := mustParse(t, `server "host"="localhost"`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error from wildcard requiring value")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("expected value count error from wildcard, got: %v", err)
	}

	// Both satisfied.
	doc = mustParse(t, `server "host"="localhost" 8080`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 23: Other-nodes-allowed with no validation on unknown nodes
func TestOtherNodesAllowedSkipsValidation(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" {}
	other-nodes-allowed #true
}
`)
	// Unknown node with properties and children — should pass.
	doc := mustParse(t, `
server
unknown "key"="value" {
	child 1 2 3
}
`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 24: Deeply nested children
func TestDeeplyNestedChildren(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "level1" {
		children {
			node "level2" {
				children {
					node "level3" {
						value {
							type "string"
						}
					}
				}
			}
		}
	}
}
`)
	// Valid nesting.
	doc := mustParse(t, `
level1 {
	level2 {
		level3 (string)"deep"
	}
}
`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid at deepest level.
	doc = mustParse(t, `
level1 {
	level2 {
		level3 (i32)42
	}
}
`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected type (string)") {
		t.Errorf("expected type error, got: %v", err)
	}
}

// Test 25: Empty document
func TestEmptyDocument(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "required-node" {
		min 1
	}
}
`)
	doc := mustParse(t, ``)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("expected cardinality error, got: %v", err)
	}
}

// Test 26: Ref-resolved constraint failure points to document location
func TestRefResolvedConstraintFailure(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "server" ref="[id=\"str-val\"]" {
	}
	definitions {
		node "server" id="str-val" {
			value {
				type "string"
			}
		}
	}
}
`)
	doc := mustParse(t, `server (i32)42`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	var vr *ValidationResult
	if !errors.As(err, &vr) {
		t.Fatal("expected *ValidationResult")
	}
	// Error should be about the document's value, not the schema definition.
	if len(vr.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	// Path should reference "server".
	path := vr.Errors[0].PathString()
	if !strings.Contains(path, "server") {
		t.Errorf("expected path to contain 'server', got: %q", path)
	}
}

// Test 27: node-names / prop-names validation
func TestNodeNamesValidation(t *testing.T) {
	sch := mustLoad(t, `
document {
	node-names {
		pattern "^[a-z-]+$"
	}
	other-nodes-allowed #true
}
`)
	// Valid names.
	doc := mustParse(t, `
good-name
another-name
`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid name.
	doc = mustParse(t, `BadName`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BadName") {
		t.Errorf("expected error mentioning 'BadName', got: %v", err)
	}
}

// Test 28: Real-world schema (integration)
func TestRealWorldSchema(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "database" {
		min 1
		max 1
		prop "name" {
			required #true
		}
		children {
			node "host" {
				value {
					format "ipv4"
				}
			}
			node "port" {
				value {
					">=" 1
					"<=" 65535
				}
			}
			node "pool" {
				children {
					node "min-connections" {
						value {
							">=" 0
						}
					}
					node "max-connections" {
						value {
							">=" 1
						}
					}
				}
			}
		}
	}
	node "auth" {
		prop "method" {
			required #true
			enum "password" "certificate" "token"
		}
	}
}
`)

	// Conforming document.
	doc := mustParse(t, `
database "name"="mydb" {
	host "192.168.1.100"
	port 5432
	pool {
		min-connections 5
		max-connections 20
	}
}
auth "method"="password"
`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Non-conforming: missing name, invalid port.
	doc = mustParse(t, `
database {
	port 99999
}
`)
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "required but missing") {
		t.Errorf("expected missing name error, got: %v", err)
	}
	if !strings.Contains(errStr, "does not satisfy") {
		t.Errorf("expected port range error, got: %v", err)
	}
}

// Test 29: Round-trip (integration)
func TestRoundTrip(t *testing.T) {
	sch := mustLoad(t, `
document {
	node "name" {
		value {
			min-length 1
		}
	}
}
`)
	// Good document.
	doc := mustParse(t, `name "hello"`)
	if err := sch.Validate(doc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Modify to be invalid (empty string).
	doc.Nodes[0].Arguments[0].SetValue("")
	err := sch.Validate(doc)
	if err == nil {
		t.Fatal("expected error after modification")
	}
	if !strings.Contains(err.Error(), "less than minimum") {
		t.Errorf("expected string length error, got: %v", err)
	}
}

// Test 30: Mini meta-schema (integration)
func TestMiniMetaSchema(t *testing.T) {
	// A schema that validates (a subset of) schema documents.
	metaSchema := mustLoad(t, `
document {
	node "document" {
		min 1
		max 1
		children {
			node "node" {
				children {
					node "prop" {
						children {
							node "required" {}
							node "type" {}
							node "enum" {}
							other-nodes-allowed #true
						}
					}
					node "value" {
						children {
							node "type" {}
							node "min" {}
							node "max" {}
							other-nodes-allowed #true
						}
					}
					node "children" {}
					node "min" {}
					node "max" {}
					other-nodes-allowed #true
				}
			}
			node "definitions" {}
			other-nodes-allowed #true
		}
	}
}
`)

	// A small schema document to validate against the meta-schema.
	smallSchema := mustParse(t, `
document {
	node "server" {
		prop "host" {
			required #true
			type "string"
		}
		value {
			type "u16"
		}
		min 1
	}
}
`)
	if err := metaSchema.Validate(smallSchema); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
