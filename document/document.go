package document

// RawSegment holds the original source bytes for a parsed element.
// When non-nil and the element has not been mutated, the generator
// can emit these bytes verbatim to preserve original formatting.
type RawSegment struct {
	Bytes []byte
}

// Document is the top-level container for a KDL document
type Document struct {
	Nodes []*Node
	// Source is the complete original input bytes, set during parsing.
	Source []byte
	// TrailingBytes holds any trailing content (whitespace, comments)
	// after the last top-level node in the original source.
	TrailingBytes []byte
}

// AddNode adds a Node to this document
func (d *Document) AddNode(child *Node) {
	d.Nodes = append(d.Nodes, child)
}

// FindNode returns the first top-level node with the given name, or nil.
func (d *Document) FindNode(name string) *Node {
	for _, n := range d.Nodes {
		if n.Name != nil && n.Name.ValueString() == name {
			return n
		}
	}
	return nil
}

// FindNodeRecursive returns the first node (DFS) with the given name, or nil.
func (d *Document) FindNodeRecursive(name string) *Node {
	for _, n := range d.Nodes {
		if n.Name != nil && n.Name.ValueString() == name {
			return n
		}
		if found := n.FindNodeRecursive(name); found != nil {
			return found
		}
	}
	return nil
}

// New cerates a new Document
func New() *Document {
	return &Document{
		Nodes: make([]*Node, 0, 32),
	}
}
