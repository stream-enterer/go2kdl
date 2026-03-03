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
	states             []parserState
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
	c.lastAddedNode = n
	return n
}

func (c *ParseContext) createNode() *document.Node {
	n := document.NewNode()
	c.node = append(c.node, n)
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

func (c *ParseContext) currentNode() *document.Node {
	if len(c.node) == 0 {
		return nil
	}
	return c.node[len(c.node)-1]
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
