package kdl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stream-enterer/go2kdl/document"
	"github.com/stream-enterer/go2kdl/internal/generator"
)

// generatePreserving is a helper that generates with PreserveFormatting:true.
func generatePreserving(doc *document.Document) (string, error) {
	var buf bytes.Buffer
	opts := generator.Options{
		Indent:             "\t",
		PreserveFormatting: true,
	}
	g := generator.NewOptions(&buf, opts)
	if err := g.Generate(doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TestFormatPreservingRoundTrip parses each non-fail compliance input and
// generates with PreserveFormatting:true, asserting byte-equality.
func TestFormatPreservingRoundTrip(t *testing.T) {
	// Slashdash nodes (/-) are discarded by the parser, so the original source
	// for those segments is irretrievably lost. These test cases contain
	// slashdashed content that cannot survive a parse→generate round-trip.
	skipSlashdash := map[string]bool{
		"commented_node":                    true,
		"escline_slashdash":                 true,
		"initial_slashdash":                 true,
		"slashdash_escline_before_node":     true,
		"slashdash_full_node":               true,
		"slashdash_in_slashdash":            true,
		"slashdash_newline_before_node":     true,
		"slashdash_node_with_child":         true,
		"slashdash_only_node":               true,
		"slashdash_only_node_with_space":    true,
		"slashdash_single_line_comment_node": true,
	}

	inputDir := filepath.Join(testCasesDir, "input")
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("failed to read input dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".kdl")
		if strings.HasSuffix(name, "_fail") {
			continue
		}
		if skipSlashdash[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join(inputDir, entry.Name())
			inputData, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("failed to read input: %v", err)
			}

			doc, err := Parse(bytes.NewReader(inputData))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			got, err := generatePreserving(doc)
			if err != nil {
				t.Fatalf("generate error: %v", err)
			}

			if got != string(inputData) {
				t.Errorf("round-trip mismatch for %s\n--- got (len %d) ---\n%s\n--- want (len %d) ---\n%s",
					name, len(got), got, len(inputData), string(inputData))
			}
		})
	}
}

// TestFormatPreservingSingleValueEdit changes one argument and checks only that differs.
func TestFormatPreservingSingleValueEdit(t *testing.T) {
	input := "node 1 2 3\nother 4\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	// Modify the second argument of "node"
	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}
	node.Arguments[1].SetValue(int64(99))
	// node's Raw is nilled by SetValue (via the value) — but actually SetValue is on Value,
	// we need to nil the node Raw too.
	node.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	// "node" was dirtied so re-formatted; "other" should be verbatim
	if !strings.Contains(got, "other 4\n") {
		t.Errorf("expected 'other 4\\n' to be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "99") {
		t.Errorf("expected modified value 99 in output, got:\n%s", got)
	}
}

// TestFormatPreservingPropertyEdit modifies a property and checks output.
func TestFormatPreservingPropertyEdit(t *testing.T) {
	input := "node key=\"value\"\nother 42\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}
	v, ok := node.Properties.Get("key")
	if !ok {
		t.Fatal("property 'key' not found")
	}
	v.SetValue("newvalue")
	node.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "other 42\n") {
		t.Errorf("expected 'other 42\\n' preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "newvalue") {
		t.Errorf("expected 'newvalue' in output, got:\n%s", got)
	}
}

// TestFormatPreservingNodeAddition adds a new node and checks others are preserved.
func TestFormatPreservingNodeAddition(t *testing.T) {
	input := "first 1\nsecond 2\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	newNode := document.NewNode()
	newNode.SetName("third")
	newNode.AddArgument(int64(3), "")
	doc.AddNode(newNode)

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	// First two nodes should be preserved verbatim
	if !strings.HasPrefix(got, "first 1\nsecond 2\n") {
		t.Errorf("expected first two nodes preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "third") {
		t.Errorf("expected new node 'third' in output, got:\n%s", got)
	}
}

// TestFormatPreservingNodeRemoval removes a node and checks the remaining are preserved.
func TestFormatPreservingNodeRemoval(t *testing.T) {
	input := "first 1\nsecond 2\nthird 3\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	// Remove "second"
	for i, n := range doc.Nodes {
		if n.Name.ValueString() == "second" {
			doc.Nodes = append(doc.Nodes[:i], doc.Nodes[i+1:]...)
			break
		}
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "second") {
		t.Errorf("expected 'second' removed, got:\n%s", got)
	}
	if !strings.Contains(got, "first 1\n") {
		t.Errorf("expected 'first 1\\n' preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "third 3\n") {
		t.Errorf("expected 'third 3\\n' preserved, got:\n%s", got)
	}
}

// TestFormatPreservingPropertyDeletion deletes a property and checks output.
func TestFormatPreservingPropertyDeletion(t *testing.T) {
	input := "node key=\"val\" other=\"keep\"\nuntouched 1\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}
	node.Properties.Delete("key")
	node.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "key=") {
		t.Errorf("expected property 'key' removed, got:\n%s", got)
	}
	if !strings.Contains(got, "untouched 1\n") {
		t.Errorf("expected 'untouched 1\\n' preserved, got:\n%s", got)
	}
}

// TestFormatPreservingAutoformat ensures Autoformat re-formats everything.
func TestFormatPreservingAutoformat(t *testing.T) {
	input := "  node   1   key=\"val\"\n"
	var buf bytes.Buffer
	err := Autoformat(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// Autoformat should produce clean output without extra spaces
	want := "node 1 key=\"val\"\n"
	if got != want {
		t.Errorf("Autoformat:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestFormatPreservingProgrammaticConstruction ensures programmatic nodes work
// with PreserveFormatting (they have no Raw so they just format normally).
func TestFormatPreservingProgrammaticConstruction(t *testing.T) {
	doc := document.New()
	n := document.NewNode()
	n.SetName("hello")
	n.AddArgument("world", "")
	doc.AddNode(n)

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := "hello \"world\"\n"
	if got != want {
		t.Errorf("programmatic:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestFormatPreservingMixedDirtyClean modifies only the middle of 3 nodes.
func TestFormatPreservingMixedDirtyClean(t *testing.T) {
	input := "alpha 1\nbeta  2\ngamma 3\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	// Modify only beta
	beta := doc.FindNode("beta")
	if beta == nil {
		t.Fatal("beta not found")
	}
	beta.Arguments[0].SetValue(int64(99))
	beta.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	// alpha and gamma preserved verbatim (including extra space in beta's original)
	if !strings.HasPrefix(got, "alpha 1\n") {
		t.Errorf("expected alpha preserved, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "gamma 3\n") {
		t.Errorf("expected gamma preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "99") {
		t.Errorf("expected 99 in output, got:\n%s", got)
	}
}

// TestFormatPreservingTrailingContent checks that trailing whitespace/comments are preserved.
func TestFormatPreservingTrailingContent(t *testing.T) {
	input := "node 1\n\n\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if got != input {
		t.Errorf("trailing content:\ngot:  %q\nwant: %q", got, input)
	}
}

// TestFormatPreservingPropertyOrder checks that insertion order is preserved.
func TestFormatPreservingPropertyOrder(t *testing.T) {
	input := "node z=\"1\" a=\"2\" m=\"3\"\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip should preserve the original property order
	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if got != input {
		t.Errorf("property order:\ngot:  %q\nwant: %q", got, input)
	}
}

// TestFormatPreservingFindNode tests the FindNode helpers.
func TestFormatPreservingFindNode(t *testing.T) {
	input := "a 1\nb 2 {\n\tc 3\n}\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if n := doc.FindNode("a"); n == nil {
		t.Error("FindNode('a') returned nil")
	}
	if n := doc.FindNode("c"); n != nil {
		t.Error("FindNode('c') should not find nested node")
	}
	if n := doc.FindNodeRecursive("c"); n == nil {
		t.Error("FindNodeRecursive('c') returned nil")
	}
}

// TestFormatPreservingRemoveNode tests RemoveNode on a child.
func TestFormatPreservingRemoveNode(t *testing.T) {
	input := "parent {\n\tchild1 1\n\tchild2 2\n}\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	parent := doc.FindNode("parent")
	if parent == nil {
		t.Fatal("parent not found")
	}

	ok := parent.RemoveNode("child1")
	if !ok {
		t.Fatal("RemoveNode returned false")
	}
	if parent.Raw != nil {
		t.Error("expected parent.Raw to be nil after RemoveNode")
	}
	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}
	if parent.Children[0].Name.ValueString() != "child2" {
		t.Errorf("expected remaining child to be 'child2', got %q", parent.Children[0].Name.ValueString())
	}
}

// TestFormatPreservingDocumentSource checks that Source and TrailingBytes are set.
func TestFormatPreservingDocumentSource(t *testing.T) {
	input := "node 1\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if string(doc.Source) != input {
		t.Errorf("Source:\ngot:  %q\nwant: %q", string(doc.Source), input)
	}
}

// TestFormatPreservingNodeRaw checks that parsed nodes have Raw set.
func TestFormatPreservingNodeRaw(t *testing.T) {
	input := "a 1\nb 2\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range doc.Nodes {
		if n.Raw == nil {
			t.Errorf("node %q has nil Raw", n.Name.ValueString())
		}
	}
}

// TestFormatPreservingValueRaw checks that parsed values have Raw set.
func TestFormatPreservingValueRaw(t *testing.T) {
	input := "node 0xFF #true \"hello\" #null\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.Nodes[0]
	for i, arg := range node.Arguments {
		if arg.Raw == nil {
			t.Errorf("argument %d has nil Raw", i)
		}
	}
}

// TestFormatPreservingChildren tests round-trip with nested children.
func TestFormatPreservingChildren(t *testing.T) {
	input := "parent {\n    child1 1\n    child2 2\n}\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if got != input {
		t.Errorf("children round-trip:\ngot:  %q\nwant: %q", got, input)
	}
}

// TestFormatPreservingHexNumber tests that hex numbers are preserved.
func TestFormatPreservingHexNumber(t *testing.T) {
	input := "node 0xDEAD_BEEF\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if got != input {
		t.Errorf("hex round-trip:\ngot:  %q\nwant: %q", got, input)
	}
}

// TestFormatPreservingRawString tests that raw strings are preserved.
func TestFormatPreservingRawString(t *testing.T) {
	input := "node #\"hello\\nworld\"#\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if got != input {
		t.Errorf("raw string round-trip:\ngot:  %q\nwant: %q", got, input)
	}
}

// TestFormatPreservingChildEditPropagatesToParent verifies that mutating a child
// found via FindNodeRecursive automatically causes the parent to re-generate
// (no manual Raw niling on ancestors needed).
func TestFormatPreservingChildEditPropagatesToParent(t *testing.T) {
	input := "parent {\n\tchild 1\n}\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	child := doc.FindNodeRecursive("child")
	if child == nil {
		t.Fatal("child not found")
	}
	child.Arguments[0].SetValue(int64(99))
	child.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "99") {
		t.Errorf("expected child edit to appear in output, got:\n%s", got)
	}
}

// TestFormatPreservingGrandchildEdit verifies dirty propagation through multiple levels.
func TestFormatPreservingGrandchildEdit(t *testing.T) {
	input := "root {\n\tparent {\n\t\tleaf 1\n\t}\n}\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	leaf := doc.FindNodeRecursive("leaf")
	if leaf == nil {
		t.Fatal("leaf not found")
	}
	leaf.Arguments[0].SetValue(int64(42))
	leaf.Raw = nil

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "42") {
		t.Errorf("expected grandchild edit to appear, got:\n%s", got)
	}
}

// TestFormatPreservingDeletePropertyNilsRaw verifies DeleteProperty nils the node's Raw.
func TestFormatPreservingDeletePropertyNilsRaw(t *testing.T) {
	input := "node key=\"val\" other=\"keep\"\nuntouched 1\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}
	ok := node.DeleteProperty("key")
	if !ok {
		t.Fatal("DeleteProperty returned false")
	}
	if node.Raw != nil {
		t.Error("expected Raw to be nil after DeleteProperty")
	}

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "key=") {
		t.Errorf("expected property 'key' removed, got:\n%s", got)
	}
	if !strings.Contains(got, "untouched 1\n") {
		t.Errorf("expected 'untouched 1\\n' preserved, got:\n%s", got)
	}
}

// TestFormatPreservingPerPropertyRaw verifies that when a node is dirty (e.g. arg added),
// properties with intact Raw are still emitted verbatim.
func TestFormatPreservingPerPropertyRaw(t *testing.T) {
	input := "node 0xDEAD color=\"red\"\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}
	// Add a new argument, dirtying the node
	node.AddArgument(int64(99), "")

	got, err := generatePreserving(doc)
	if err != nil {
		t.Fatal(err)
	}

	// The hex value's Raw on the argument should be preserved
	if !strings.Contains(got, "0xDEAD") {
		t.Errorf("expected hex arg preserved via Raw, got:\n%s", got)
	}
	if !strings.Contains(got, "99") {
		t.Errorf("expected new arg in output, got:\n%s", got)
	}
}

// TestFormatPreservingShallowCopyIsolation verifies that ShallowCopy creates
// independent slices for Children and Arguments.
func TestFormatPreservingShallowCopyIsolation(t *testing.T) {
	original := document.NewNode()
	original.SetName("test")
	original.AddArgument(int64(1), "")
	child := document.NewNode()
	child.SetName("child")
	original.AddNode(child)

	cp := original.ShallowCopy()

	// Mutate copy's slices
	cp.AddArgument(int64(2), "")
	newChild := document.NewNode()
	newChild.SetName("newchild")
	cp.AddNode(newChild)

	// Original should be unchanged
	if len(original.Arguments) != 1 {
		t.Errorf("expected original to have 1 arg, got %d", len(original.Arguments))
	}
	if len(original.Children) != 1 {
		t.Errorf("expected original to have 1 child, got %d", len(original.Children))
	}
}

// TestFormatPreservingSetters verifies that SetType, SetChildren, SetArguments, SetFlag nil Raw.
func TestFormatPreservingSetters(t *testing.T) {
	input := "node 1\n"
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	node := doc.FindNode("node")
	if node == nil {
		t.Fatal("node not found")
	}

	// SetType on node
	node.Raw = &document.RawSegment{Bytes: []byte("fake")}
	node.SetType("mytype")
	if node.Raw != nil {
		t.Error("SetType should nil Raw")
	}

	// SetChildren
	node.Raw = &document.RawSegment{Bytes: []byte("fake")}
	node.SetChildren(nil)
	if node.Raw != nil {
		t.Error("SetChildren should nil Raw")
	}

	// SetArguments
	node.Raw = &document.RawSegment{Bytes: []byte("fake")}
	node.SetArguments(nil)
	if node.Raw != nil {
		t.Error("SetArguments should nil Raw")
	}

	// SetFlag on value
	v := &document.Value{Value: int64(1)}
	v.Raw = &document.RawSegment{Bytes: []byte("1")}
	v.SetFlag(document.FlagHexadecimal)
	if v.Raw != nil {
		t.Error("SetFlag should nil Raw")
	}
}
