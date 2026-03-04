package document

import (
	"bytes"
	"io"
	"strings"

	"github.com/stream-enterer/go2kdl/internal/tokenizer"
)

// TypeAnnotation represents a type annotation in a KDL document
type TypeAnnotation string

type Comment struct {
	// Before specifies a comment that appears before the node, if non-nil
	Before []byte
	// After specifies a comment that appears after the node, if non-nil
	After []byte
}

// Node represents a single node in a KDL document
type Node struct {
	// Name is name of the node
	Name *Value
	// Type is the type annotation of the node, or an empty string if none
	Type TypeAnnotation
	// Arguments is the list of arguments for the node, or nil if none
	Arguments []*Value
	// Properties is the list of properties for the node, or nil if none
	Properties Properties
	// Children is the list of child nodes for the node, or nil if none
	Children []*Node
	// Comment is the comment for the node, or nil if none
	Comment *Comment
	// Span is the position of the node name in source (zero if unavailable,
	// e.g. programmatically constructed nodes).
	Span Span
	// Raw holds the original source text for this node (including leading trivia,
	// arguments, properties, children block, and terminator). When non-nil and the
	// node has not been mutated, the generator emits these bytes verbatim.
	Raw *RawSegment
}

func (n *Node) ShallowCopy() *Node {
	r := &Node{}
	*r = *n
	return r
}

func (n *Node) ExpectChildren(count int) {
	want := len(n.Children) + count
	if cap(n.Children) < want {
		c := make([]*Node, 0, want)
		c = append(c, n.Children...)
		n.Children = c
	}
}

func (n *Node) ExpectArguments(count int) {
	want := len(n.Arguments) + count
	if cap(n.Arguments) < want {
		a := make([]*Value, 0, want)
		a = append(a, n.Arguments...)
		n.Arguments = a
	}
}

// AddNode adds a Node as a child of this node
func (n *Node) AddNode(child *Node) {
	n.Children = append(n.Children, child)
	n.Raw = nil
}

// FindNode returns the first direct child with the given name, or nil.
func (n *Node) FindNode(name string) *Node {
	for _, child := range n.Children {
		if child.Name != nil && child.Name.ValueString() == name {
			return child
		}
	}
	return nil
}

// FindNodeRecursive returns the first descendant (DFS) with the given name, or nil.
func (n *Node) FindNodeRecursive(name string) *Node {
	for _, child := range n.Children {
		if child.Name != nil && child.Name.ValueString() == name {
			return child
		}
		if found := child.FindNodeRecursive(name); found != nil {
			return found
		}
	}
	return nil
}

// RemoveNode removes the first direct child with the given name and returns true,
// or returns false if no child with that name exists. Nils the parent's Raw.
func (n *Node) RemoveNode(name string) bool {
	for i, child := range n.Children {
		if child.Name != nil && child.Name.ValueString() == name {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			n.Raw = nil
			return true
		}
	}
	return false
}

// NewNode creates and returns a new Node
func NewNode() *Node {
	return &Node{}
}

// SetName sets the name of the node
func (n *Node) SetName(name string) {
	n.Name = &Value{Value: name}
	n.Raw = nil
}

// SetNameToken sets the name of the node from a Token, and returns a non-nil error on failure
func (n *Node) SetNameToken(t tokenizer.Token) error {
	v, err := ValueFromToken(t)
	if err != nil {
		return err
	}
	n.Name = v
	n.Span = v.Span
	n.Raw = nil
	return nil
}

// AddArgument adds an argument to this node, with the given type annotation (which may be ""), and returns the added
// Value
func (n *Node) AddArgument(value interface{}, typeAnnot TypeAnnotation) *Value {
	v := &Value{
		Value: value,
		Type:  typeAnnot,
	}
	n.Arguments = append(n.Arguments, v)
	n.Raw = nil
	return v
}

// AddArgumentToken adds an argument to this node from a Token, with the given type annotation (which may be ""), and
// returns a non-nil error on failure
func (n *Node) AddArgumentToken(t tokenizer.Token, typeAnnot tokenizer.Token) error {
	v, err := ValueFromToken(t)
	if err != nil {
		return err
	}
	if typeAnnot.Valid() {
		v.Type = TypeAnnotation(typeAnnot.Data)
	}
	n.Arguments = append(n.Arguments, v)
	n.Raw = nil
	return nil
}

// AddProperty adds a property to this node with the given name, value, and type annotation (which may be ""), and
// returns the added Value
func (n *Node) AddProperty(name string, value interface{}, typeAnnot TypeAnnotation) *Value {
	v := &Value{
		Type:  typeAnnot,
		Value: value,
		Flag:  0,
	}
	if !n.Properties.Allocated() {
		n.Properties.Alloc()
	}
	n.Properties.Add(name, v)
	n.Raw = nil
	return v
}

func (n *Node) AddPropertyValue(name string, value *Value, typeAnnot TypeAnnotation) *Value {
	if !n.Properties.Allocated() {
		n.Properties.Alloc()
	}
	n.Properties.Add(name, value)
	n.Raw = nil
	return value
}

// AddPropertyToken adds a property to this node from the given name and value Token and type annotation (which may be
// "") and returns the added Value and a non-nil error on failure
func (n *Node) AddPropertyToken(name tokenizer.Token, value tokenizer.Token, typeAnnot tokenizer.Token) (*Value, error) {
	nt, err := ValueFromToken(name)
	if err != nil {
		return nil, err
	}
	vt, err := ValueFromToken(value)
	if err != nil {
		return nil, err
	}
	if typeAnnot.Valid() {
		vt.Type = TypeAnnotation(typeAnnot.Data)
	}

	if !n.Properties.Allocated() {
		n.Properties.Alloc()
	}
	n.Properties.Add(nt.ValueString(), vt)
	n.Raw = nil

	return vt, nil
}

// NodeWriteOptions controls how a node is written using WriteToOptions.
type NodeWriteOptions struct {
	// LeadingTrailingSpace specifies whether leading space (indentation) and newlines are included in the output
	LeadingTrailingSpace bool
	// NameAndType specifies whether the node's name and type annotation are included in the output
	NameAndType bool
	// Depth specifies the indentation depth
	Depth int
	// Indent specifies the byte string to use for each indentation level
	Indent []byte
	// IgnoreFlags specifies that the formatting flags for the node's value(s) should be ignored
	IgnoreFlags bool
	// AddSemicolons causes lines to be terminated with semicolons
	AddSemicolons bool
	// AddEquals causes '=' symbols to be inserted between nodes and their values, which is noncompliant with the KDL spec
	AddEquals bool
	// AddEquals causes ':' symbols to be inserted between nodes and their values, which is noncompliant with the KDL spec
	AddColons bool
	// PreserveFormatting emits Raw source bytes for values that haven't been mutated
	PreserveFormatting bool
}

var defaultNodeWriteOptions = NodeWriteOptions{
	LeadingTrailingSpace: false,
	NameAndType:          true,
	Depth:                0,
	Indent:               []byte{'\t'},
	IgnoreFlags:          false,
}

// String returns the complete KDL representation of this node, including its type annotation and name
func (n *Node) String() string {
	b := strings.Builder{}
	_, _ = n.WriteTo(&b)
	return b.String()
}

// ValueString returns the KDL representation of this node, without its type annotation or name.
func (n *Node) ValueString() string {
	b := strings.Builder{}
	_, _ = n.WriteValueTo(&b)
	return b.String()
}

// TextString returns a text representation of this node, without its type annotation or name. If the node contains
// exactly one argument, zero properties, and zero children, it writes the unquoted string representation of the only
// argument.
func (n *Node) TextString() string {
	b := strings.Builder{}
	_, _ = n.WriteTextValueTo(&b)
	return b.String()
}

// WriteValueTo writes the KDL representation of this node, without its type annotation or name.
func (n *Node) WriteValueTo(w io.Writer) (int64, error) {
	opts := defaultNodeWriteOptions
	opts.NameAndType = false
	return n.WriteToOptions(w, opts)
}

// WriteTextValueTo writes a text representation of the arguments, properties, and children of node. If node contains
// exactly one argument, zero properties, and zero children, it writes the unquoted string representation of the only
// argument.
func (n *Node) WriteTextValueTo(w io.Writer) (int64, error) {
	if len(n.Arguments) == 1 && n.Properties.Len() == 0 && len(n.Children) == 0 {
		nw, err := w.Write([]byte(n.Arguments[0].ValueString()))
		return int64(nw), err
	}

	nw, err := n.WriteToOptions(w, NodeWriteOptions{
		LeadingTrailingSpace: false,
		NameAndType:          false,
		Depth:                0,
		Indent:               []byte{},
		IgnoreFlags:          false,
	})
	return int64(nw), err

}

// WriteTo writes the complete KDL representation of this node, including its type annotation or name.
func (n *Node) WriteTo(w io.Writer) (int64, error) {
	return n.WriteToOptions(w, defaultNodeWriteOptions)
}

// WriteToOptions writes the KDL representation of this node with the specified options.
func (n *Node) WriteToOptions(w io.Writer, opts NodeWriteOptions) (int64, error) {
	var (
		nw  int64
		err error
	)
	write := func(b []byte) {
		n, e := w.Write(b)
		nw += int64(n)
		err = e
	}

	var indent []byte
	if opts.Depth > 0 && opts.LeadingTrailingSpace {
		indent = bytes.Repeat(opts.Indent, opts.Depth)
	}

	if n.Comment != nil {
		if n.Comment.Before != nil {
			// println("BEFORE [" + string(n.Comment.Before) + "]")
			comment := bytes.Trim(n.Comment.Before, " \t")
			lines := bytes.Split(comment, []byte{'\n'})

			newlineCount := 0
			for _, line := range lines {
				line = bytes.TrimSpace(line)
				if len(line) > 0 {
					write(indent)
					write(line)
					newlineCount = 0
				} else {
					newlineCount++
				}
				if newlineCount < 2 {
					write([]byte{'\n'})
				}
			}
		}
	}

	if opts.Depth > 0 && opts.LeadingTrailingSpace {
		write(indent)
	}
	if opts.NameAndType {
		if len(n.Type) > 0 {
			if err == nil {
				write([]byte{'('})
			}
			if err == nil {
				write([]byte(n.Type))
			}
			if err == nil {
				write([]byte{')'})
			}
		}
		if err == nil {
			// node names don't need to be quoted unless they include non-Identifier characters
			write([]byte(n.Name.NodeNameString()))
		}
	}

	if opts.AddEquals && len(n.Arguments) > 0 && !n.Properties.Exist() && len(n.Children) == 0 {
		write([]byte{' ', '='})
	} else if opts.AddColons && len(n.Arguments) > 0 && !n.Properties.Exist() && len(n.Children) == 0 {
		write([]byte{':'})
	}

	for i, arg := range n.Arguments {
		if err == nil && (opts.NameAndType || i > 0) {
			write([]byte{' '})
		}
		if err == nil {
			if opts.PreserveFormatting && arg.Raw != nil {
				if len(arg.Type) > 0 {
					write([]byte{'('})
					write([]byte(arg.Type))
					write([]byte{')'})
				}
				write(arg.Raw.Bytes)
			} else if opts.IgnoreFlags {
				write([]byte(arg.UnformattedString()))
			} else {
				write([]byte(arg.FormattedString()))
			}
		}
	}
	if n.Properties.Exist() && err == nil {
		if opts.IgnoreFlags {
			write([]byte(n.Properties.UnformattedString()))
		} else {
			write([]byte(n.Properties.String()))
		}
	}
	if err == nil {
		if len(n.Children) > 0 {
			write([]byte{' ', '{', '\n'})

			opts.Depth++
			if err == nil {
				for _, n := range n.Children {
					if nnw, err := n.WriteToOptions(w, opts); err != nil {
						break
					} else {
						nw += nnw
					}
				}
			}
			opts.Depth--

			if opts.Depth > 0 && err == nil {
				write(bytes.Repeat(opts.Indent, opts.Depth))
			}
			if err == nil {
				write([]byte{'}'})
			}
		} else if opts.AddSemicolons {
			write([]byte{';'})
		}
	}

	if err == nil {
		if n.Comment != nil && n.Comment.After != nil {
			comment := bytes.Trim(n.Comment.After, " \t")
			lines := bytes.Split(comment, []byte{'\n'})

			for _, line := range lines {
				write(indent)
				write(bytes.TrimSpace(line))
				write([]byte{'\n'})
			}
		} else if opts.LeadingTrailingSpace {
			write([]byte{'\n'})
		}

	}

	return nw, err
}
