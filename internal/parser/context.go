package parser

import (
	"bytes"
	"errors"

	"github.com/ar-go/go2kdl/document"
	"github.com/ar-go/go2kdl/internal/tokenizer"
	"github.com/ar-go/go2kdl/relaxed"
)

type ParseFlags uint8

func (p ParseFlags) Has(f ParseFlags) bool {
	return (p & f) != 0
}

const (
	ParseComments ParseFlags = 1 << iota
)

type ParseContextOptions struct {
	RelaxedNonCompliant relaxed.Flags
	Flags               ParseFlags
}

var defaultParseContextOptions = ParseContextOptions{
	RelaxedNonCompliant: 0,
}

// ParseContext maintains the parser context for a KDL document
type ParseContext struct {
	// document being generated
	doc *document.Document
	// state stack; current state is pushed onto this when a child block is encountered
	states              []parserState
	childBlockSeenStack []bool
	// current state
	state parserState
	// node stack; new nodes are pushed onto this when a child block is encountered; current node is last
	node []*document.Node
	// temporary storage for identifier (usually node name or property key)
	ident tokenizer.Token
	// temporary storage for type annotation
	typeAnnot tokenizer.Token
	// true if a continuation backslash has been encountered and the next newline should be ignored
	continuation bool
	// true if a /- was encountered and the next entire node should be ignored
	ignoreNextNode bool
	// true if a /- was encountered and the next arg/prop should be ignored
	ignoreNextArgProp bool
	// true if a /- was encountered and the next child block should be ignored
	ignoreChildren int
	// true if a child block (including slashdashed) has been seen on the current node
	childBlockSeen bool
	opts           ParseContextOptions

	comment pendingComment

	lastAddedNode *document.Node
	recent        recentTokens

	// source is the raw input buffer, used to populate error Source fields.
	source []byte

	// nodeStartOffsets is a stack of byte offsets where each node's raw segment begins
	// (i.e. the position right after the previous node's terminator, capturing leading trivia).
	nodeStartOffsets []int
	// lastNodeEnd is the byte offset right after the last completed node's terminator.
	lastNodeEnd int
	// lastNodeEndStack saves/restores lastNodeEnd when entering/leaving children blocks.
	lastNodeEndStack []int
	// nodeIgnored tracks whether each stacked node was slashdashed (ignored) and thus
	// should not receive a Raw segment.
	nodeIgnored []bool
}

type pendingComment struct {
	bytes.Buffer
}

func (p pendingComment) CopyBytes() []byte {
	if p.Len() == 0 {
		return nil
	}

	r := make([]byte, p.Len())
	copy(r, p.Bytes())
	return r
}

func (c *ParseContext) RelaxedNonCompliant() relaxed.Flags {
	return c.opts.RelaxedNonCompliant
}

// SetSource sets the raw input buffer for error diagnostics.
func (c *ParseContext) SetSource(src []byte) {
	c.source = src
}

// Document returns the current parsed document
func (c *ParseContext) Document() *document.Document {
	return c.doc
}

func (c *ParseContext) addNode() *document.Node {
	n := document.NewNode()
	if len(c.node) > 0 {
		c.node[len(c.node)-1].AddNode(n)
	} else {
		c.doc.AddNode(n)
	}
	c.node = append(c.node, n)
	c.nodeStartOffsets = append(c.nodeStartOffsets, c.lastNodeEnd)
	c.nodeIgnored = append(c.nodeIgnored, false)
	c.lastAddedNode = n
	return n
}

func (c *ParseContext) createNode() *document.Node {
	n := document.NewNode()
	c.node = append(c.node, n)
	c.nodeStartOffsets = append(c.nodeStartOffsets, c.lastNodeEnd)
	c.nodeIgnored = append(c.nodeIgnored, true) // ignored/slashdashed node
	c.lastAddedNode = n
	return n
}

var errNodeStackEmpty = errors.New("node stack empty")

func (c *ParseContext) popNode() (*document.Node, error) {
	if len(c.node) == 0 {
		return nil, errNodeStackEmpty
	}
	node := c.currentNode()
	c.node = c.node[0 : len(c.node)-1]
	// pop auxiliary stacks
	if len(c.nodeStartOffsets) > 0 {
		c.nodeStartOffsets = c.nodeStartOffsets[:len(c.nodeStartOffsets)-1]
	}
	if len(c.nodeIgnored) > 0 {
		c.nodeIgnored = c.nodeIgnored[:len(c.nodeIgnored)-1]
	}
	return node, nil
}

func (c *ParseContext) popNodeAndState() (parserState, *document.Node, error) {
	ps, err := c.popState()
	if err != nil {
		return ps, nil, err
	}
	node, err := c.popNode()
	return ps, node, err
}

// popNodeAndStateAt pops the current node and state, and if the node was not
// slashdashed, sets its Raw segment from source[startOffset:endOffset].
func (c *ParseContext) popNodeAndStateAt(endOffset int) (parserState, *document.Node, error) {
	// Read start offset and ignored flag before popNode removes them
	var startOffset int
	var ignored bool
	if n := len(c.nodeStartOffsets); n > 0 {
		startOffset = c.nodeStartOffsets[n-1]
	}
	if n := len(c.nodeIgnored); n > 0 {
		ignored = c.nodeIgnored[n-1]
	}

	ps, err := c.popState()
	if err != nil {
		return ps, nil, err
	}
	node, err := c.popNode()
	if err != nil {
		return ps, node, err
	}

	// Set Raw on non-ignored nodes when source is available
	if !ignored && len(c.source) > 0 && endOffset > startOffset && endOffset <= len(c.source) {
		node.Raw = &document.RawSegment{Bytes: c.source[startOffset:endOffset]}
	}
	c.lastNodeEnd = endOffset
	return ps, node, nil
}

// LastNodeEnd returns the byte offset after the last completed node.
func (c *ParseContext) LastNodeEnd() int {
	return c.lastNodeEnd
}

// tokenEndOffset returns the byte offset after the given token.
// For EOF tokens (which have zero offset/data), it returns the source length.
func (c *ParseContext) tokenEndOffset(t tokenizer.Token) int {
	if t.ID == tokenizer.EOF {
		return len(c.source)
	}
	return t.Offset + len(t.Data)
}

func (c *ParseContext) currentNode() *document.Node {
	if len(c.node) == 0 {
		return nil
	}
	return c.node[len(c.node)-1]
}

// enterChildrenBlock saves the current lastNodeEnd and sets it to the byte after
// the opening brace.
func (c *ParseContext) enterChildrenBlock(braceOffset int) {
	c.lastNodeEndStack = append(c.lastNodeEndStack, c.lastNodeEnd)
	c.lastNodeEnd = braceOffset + 1
}

// leaveChildrenBlock restores lastNodeEnd from the stack.
func (c *ParseContext) leaveChildrenBlock() {
	if n := len(c.lastNodeEndStack); n > 0 {
		c.lastNodeEnd = c.lastNodeEndStack[n-1]
		c.lastNodeEndStack = c.lastNodeEndStack[:n-1]
	}
}

func (c *ParseContext) pushState(newState parserState) {
	c.states = append(c.states, c.state)
	c.childBlockSeenStack = append(c.childBlockSeenStack, c.childBlockSeen)
	c.state = newState
	// Reset childBlockSeen for the new state context (e.g., inside a child block,
	// child nodes start fresh without having seen any child blocks)
	if newState == stateChildren {
		c.childBlockSeen = false
	}
}

var errStateStackEmpty = errors.New("state stack empty")

func (c *ParseContext) popState() (parserState, error) {
	if len(c.states) == 0 {
		return c.state, errStateStackEmpty
	}
	c.state = c.states[len(c.states)-1]
	c.states = c.states[0 : len(c.states)-1]
	if len(c.childBlockSeenStack) > 0 {
		c.childBlockSeen = c.childBlockSeenStack[len(c.childBlockSeenStack)-1]
		c.childBlockSeenStack = c.childBlockSeenStack[0 : len(c.childBlockSeenStack)-1]
	}
	return c.state, nil
}
