package parser

import (
	"bytes"
	"fmt"

	"github.com/ar-go/go2kdl/document"
	"github.com/ar-go/go2kdl/internal/tokenizer"
	"github.com/ar-go/go2kdl/relaxed"
)

type stateTransitionFunc func(*ParseContext, tokenizer.Token) error

// stateTransitions maps a given parser state to the tokens allowed in that state, and provides a transition function
// that accepts a token and a context, processes the token, and updates the parser state
//
// TODO: benchmark this; it's likely faster (though likely much less readable) to do this using switch statements
var stateTransitions = map[parserState]map[tokenizer.TokenID]stateTransitionFunc{
	stateDocument: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: whitespace between type annotation and identifier is allowed
			return nil
		},
		tokenizer.ClassIdentifier: func(c *ParseContext, t tokenizer.Token) error {
			// an Identifier in the outermost context is always a node declaration
			var node *document.Node
			if c.ignoreNextNode {
				node = c.createNode()
				c.ignoreNextNode = false
			} else {
				node = c.addNode()
			}

			if err := node.SetNameToken(t); err != nil {
				return err
			}

			if c.opts.Flags.Has(ParseComments) {
				c.comment.Write(c.recent.TrailingNewlines())
				if c.comment.Len() > 0 {
					node.Comment = &document.Comment{
						Before: bytes.TrimSuffix(c.comment.CopyBytes(), []byte{'\n'}),
					}
					c.comment.Reset()
				}
			}
			if c.typeAnnot.Valid() {
				node.Type = document.TypeAnnotation(c.typeAnnot.Data)
				c.typeAnnot.Clear()
			}
			c.pushState(stateNode)
			return nil
		},
		tokenizer.ParensOpen: func(c *ParseContext, t tokenizer.Token) error {
			// a ( in the outermost context is the beginning of a type annotation for a node
			c.pushState(stateTypeAnnot)
			return nil
		},
		tokenizer.ClassTerminator: func(c *ParseContext, t tokenizer.Token) error {
			if c.typeAnnot.Valid() {
				return fmt.Errorf("expected value after type, found %s in state %s", t.ID, c.state)
			}

			// KDLv2: /- must have something to comment out before EOF
			if c.ignoreNextNode && t.ID == tokenizer.EOF {
				return fmt.Errorf("dangling slashdash with nothing to comment out")
			}

			// ignore extraneous newlines, semicolons, and EOF
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments after type annotation
			if c.opts.Flags.Has(ParseComments) {
				c.comment.Write(c.recent.TrailingNewlines())
				c.comment.Write(t.Data)
			}
			return nil
		},
		tokenizer.TokenComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: /- cannot appear after a type annotation
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected /- after type annotation")
			}
			c.ignoreNextNode = true
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
	},
	stateChildren: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: whitespace between type annotation and identifier is allowed
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			if c.opts.Flags.Has(ParseComments) {
				trailing := c.recent.TrailingNewlines()
				c.comment.Write(trailing)
				c.comment.Write(t.Data)
			}
			return nil
		},
		tokenizer.TokenComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: /- cannot appear after a type annotation
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected /- after type annotation")
			}
			c.ignoreNextNode = true
			return nil
		},
		tokenizer.ParensOpen: func(c *ParseContext, t tokenizer.Token) error {
			// a ( inside a node declaration is the beginning of a type annotation for a node
			c.pushState(stateTypeAnnot)
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.Newline: func(c *ParseContext, t tokenizer.Token) error {
			// ignore extraneous newlines
			return nil
		},
		tokenizer.Semicolon: func(c *ParseContext, t tokenizer.Token) error {
			// ignore extraneous semicolons between nodes
			return nil
		},
		tokenizer.ClassIdentifier: func(c *ParseContext, t tokenizer.Token) error {
			// an Identifier in the child context is always a node declaration
			var node *document.Node
			if c.ignoreNextNode || c.ignoreChildren > 0 {
				node = c.createNode()
				c.ignoreNextNode = false
			} else {
				node = c.addNode()
			}
			if err := node.SetNameToken(t); err != nil {
				return err
			}

			if c.opts.Flags.Has(ParseComments) {
				c.comment.Write(c.recent.TrailingNewlines())
				if c.comment.Len() > 0 {
					node.Comment = &document.Comment{
						Before: bytes.TrimSuffix(c.comment.CopyBytes(), []byte{'\n'}),
					}
					c.comment.Reset()
				}
			}
			if c.typeAnnot.Valid() {
				node.Type = document.TypeAnnotation(c.typeAnnot.Data)
				c.typeAnnot.Clear()
			}
			c.pushState(stateNode)
			return nil
		},
		tokenizer.BraceClose: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: /- must have something to comment out before }
			if c.ignoreNextNode {
				return fmt.Errorf("dangling slashdash before }")
			}

			if c.ignoreChildren > 0 {
				c.ignoreChildren--
			}

			if c.opts.Flags.Has(ParseComments) {
				c.comment.Write(c.recent.TrailingNewlines())
				if c.comment.Len() > 0 {
					lastNode := c.lastAddedNode
					if lastNode.Comment == nil {
						lastNode.Comment = &document.Comment{}
					}
					lastNode.Comment.After = append(lastNode.Comment.After, bytes.TrimSuffix(c.comment.CopyBytes(), []byte{'\n'})...)
					c.comment.Reset()
				}
			}

			_, err := c.popState()
			return err
		},
	},

	stateTypeAnnot: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow whitespace before type name inside type annotation
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments inside type annotations
			return nil
		},
		tokenizer.BareIdentifier: func(c *ParseContext, t tokenizer.Token) error {
			c.typeAnnot = t
			c.state = stateTypeDone
			return nil
		},
		tokenizer.ClassString: func(c *ParseContext, t tokenizer.Token) error {
			c.typeAnnot = t
			c.state = stateTypeDone
			return nil
		},
	},
	stateTypeDone: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow whitespace after type name before closing )
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments after type name before closing )
			return nil
		},
		tokenizer.ParensClose: func(c *ParseContext, t tokenizer.Token) error {
			_, err := c.popState()
			return err
		},
	},
	stateNode: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			c.state = stateNodeParams
			return nil
		},
		tokenizer.ClassTerminator: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				return nil
			} else {
				_, _, err := c.popNodeAndState()
				return err
			}
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.BraceOpen: func(c *ParseContext, t tokenizer.Token) error {
			if c.ignoreNextArgProp || c.ignoreChildren > 0 {
				c.ignoreNextArgProp = false
				c.ignoreChildren++
			}
			c.childBlockSeen = true
			c.pushState(stateChildren)
			return nil
		},
		tokenizer.BraceClose: func(c *ParseContext, t tokenizer.Token) error {
			// End the current node
			_, _, err := c.popNodeAndState()
			if err != nil {
				return err
			}
			// Now handle the BraceClose in the parent state (should be stateChildren)
			if c.state == stateChildren {
				if c.ignoreChildren > 0 {
					c.ignoreChildren--
				}
				_, err = c.popState()
				return err
			}
			return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
		},
		tokenizer.Equals: func(c *ParseContext, t tokenizer.Token) error {
			if c.opts.RelaxedNonCompliant.Permit(relaxed.YAMLTOMLAssignments) {
				c.state = stateNodeParams
				return nil
			} else {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}
		},
	},
	stateNodeParams: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: whitespace between type annotation and value is allowed
			return nil
		},
		tokenizer.Equals: func(c *ParseContext, t tokenizer.Token) error {
			if c.opts.RelaxedNonCompliant.Permit(relaxed.YAMLTOMLAssignments) && !c.typeAnnot.Valid() && !c.ident.Valid() {
				// ignore
				return nil
			} else {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}
		},
		tokenizer.TokenComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: /- cannot appear after a type annotation
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected /- after type annotation")
			}
			c.ignoreNextArgProp = true
			return nil
		},
		tokenizer.MultiLineComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments after type annotation
			return nil
		},
		tokenizer.SingleLineComment: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				// Part of an escline - the comment is consumed along with the line
				return nil
			}
			c.state = stateNodeEnd
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.ParensOpen: func(c *ParseContext, t tokenizer.Token) error {
			// a ( inside a node declaration is hte beginning of a type annotation for a node
			c.pushState(stateTypeAnnot)
			return nil
		},
		tokenizer.BareIdentifier: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: entries cannot appear after a child block
			if c.childBlockSeen {
				return fmt.Errorf("entry found after child block")
			}
			// KDLv2: bare identifiers can be values too, so we need to check if followed by =
			c.ident = t
			c.state = stateArgProp
			return nil
		},
		tokenizer.SuffixedDecimal: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: entries cannot appear after a child block
			if c.childBlockSeen {
				return fmt.Errorf("entry found after child block")
			}
			// a suffixed identifier inside a node declaration can only be an argument
			c.typeAnnot.Clear()
			c.ident.Clear()

			if c.ignoreNextArgProp {
				c.ignoreNextArgProp = false
			} else if err := c.currentNode().AddArgumentToken(t, c.typeAnnot); err != nil {
				return err
			}

			c.state = stateNodeParams
			return nil
		},
		tokenizer.ClassString: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: entries cannot appear after a child block
			if c.childBlockSeen {
				return fmt.Errorf("entry found after child block")
			}
			// a string value inside a node declaration is either an argument or a property name; save it
			c.ident = t
			c.state = stateArgProp
			return nil
		},
		tokenizer.ClassNonStringValue: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: entries cannot appear after a child block
			if c.childBlockSeen {
				return fmt.Errorf("entry found after child block")
			}
			// a non-string value inside a node declaration is always an argument, but we save it just to make sure it isn't followed by an equal sign
			c.ident = t
			c.state = stateArgProp
			return nil
		},
		tokenizer.BraceOpen: func(c *ParseContext, t tokenizer.Token) error {
			if c.ignoreNextArgProp || c.ignoreChildren > 0 {
				c.ignoreNextArgProp = false
				c.ignoreChildren++
			}
			c.childBlockSeen = true
			c.pushState(stateChildren)
			return nil
		},
		tokenizer.ClassTerminator: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				return nil
			} else if c.ignoreNextArgProp {
				// KDLv2: /- must have something to comment out
				// EOF and semicolons mean there's nothing left to slashdash
				if t.ID == tokenizer.EOF || t.ID == tokenizer.Semicolon {
					return fmt.Errorf("dangling slashdash with nothing to comment out")
				}
				// Don't end the node - slashdash is still looking for its target (after newline)
				return nil
			} else if c.typeAnnot.Valid() {
				return fmt.Errorf("expected value after type, found %s in state %s", t.ID, c.state)
			} else {
				_, _, err := c.popNodeAndState()
				return err
			}
		},
	},
	stateNodeEnd: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			return nil
		},
		tokenizer.ClassEndOfLine: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				c.state = stateNodeParams
				return nil
			} else if c.ignoreNextArgProp {
				// Don't end the node - slashdash is still looking for its target
				c.state = stateNodeParams
				return nil
			} else {
				_, _, err := c.popNodeAndState()
				return err
			}
		},
		tokenizer.BraceClose: func(c *ParseContext, t tokenizer.Token) error {
			// End the current node
			_, _, err := c.popNodeAndState()
			if err != nil {
				return err
			}
			// Now handle the BraceClose in the parent state (should be stateChildren)
			if c.state == stateChildren {
				if c.ignoreChildren > 0 {
					c.ignoreChildren--
				}
				_, err = c.popState()
				return err
			}
			return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
		},
	},
	stateProperty: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow whitespace before = in properties
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments before = in properties
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.Equals: func(c *ParseContext, t tokenizer.Token) error {
			// cannot cannot use a type annotation on a property key
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}

			// equals is the only valid value after a bare-identifier property name
			c.state = statePropertyValue
			return nil
		},
	},
	stateArgProp: {
		tokenizer.TokenComment: func(c *ParseContext, t tokenizer.Token) error {
			if c.ignoreNextArgProp {
				c.ignoreNextArgProp = false
			} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
				return err
			}
			c.typeAnnot.Clear()
			c.ident.Clear()

			c.ignoreNextArgProp = true
			return nil
		},

		tokenizer.BraceOpen: func(c *ParseContext, t tokenizer.Token) error {
			// if we're at the end of the node and didn't find an equal sign, it was just an argument

			if c.ident.Valid() {
				// KDLv2: entries cannot appear after a child block
				if c.childBlockSeen {
					return fmt.Errorf("entry found after child block")
				}
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}

			if c.ignoreNextArgProp || c.ignoreChildren > 0 {
				c.ignoreNextArgProp = false
				c.ignoreChildren++
			}

			c.childBlockSeen = true
			c.pushState(stateChildren)
			return nil
		},
		tokenizer.Equals: func(c *ParseContext, t tokenizer.Token) error {
			// cannot use a type annotation on a property key
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}
			// KDLv2: any identifier-class token can be a property name
			// (BareIdentifier, QuotedString, RawString, MultilineString, MultilineRawString)
			isIdentifier := false
			for _, class := range c.ident.ID.Classes() {
				if class == tokenizer.ClassIdentifier {
					isIdentifier = true
					break
				}
			}
			if !isIdentifier {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}

			// equals indicates that it's a property
			c.state = statePropertyValue
			return nil
		},

		tokenizer.Whitespace: func(c *ParseContext, p tokenizer.Token) error {
			// KDLv2: whitespace after ident could still be followed by = (space around equals)
			// Transition to stateArgPropPostWS to resolve the ambiguity
			c.state = stateArgPropPostWS
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			// Line continuation after a value - semantically like whitespace
			// The ident could still be an arg or prop key, so transition to post-WS state
			c.continuation = true
			c.state = stateArgPropPostWS
			return nil
		},

		tokenizer.ClassTerminator: func(c *ParseContext, t tokenizer.Token) error {
			if c.ident.Valid() {
				// if we're at the end of the node and have an identifier but didn't find an equal sign, it was just an argument
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}

			// and the node is done
			_, _, err := c.popNodeAndState()
			return err
		},
		tokenizer.BraceClose: func(c *ParseContext, t tokenizer.Token) error {
			// Commit ident as argument
			if c.ident.Valid() {
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}
			// End the current node
			_, _, err := c.popNodeAndState()
			if err != nil {
				return err
			}
			// Now handle the BraceClose in the parent state (should be stateChildren)
			if c.state == stateChildren {
				if c.ignoreChildren > 0 {
					c.ignoreChildren--
				}
				_, err = c.popState()
				return err
			}
			return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
		},
		tokenizer.ClassValue: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: if we have a pending ident (meaning we just saw a value without whitespace),
			// this means two values without whitespace separation, which is invalid.
			// But if ident was cleared (by /- handler), the value is the slashdash target.
			if c.ident.Valid() {
				return fmt.Errorf("values must be separated by whitespace")
			}
			// /- was just seen and cleared ident; this is the slashdash target.
			// Save it and process normally (ignoreNextArgProp will cause it to be discarded)
			c.ident = t
			return nil
		},
	},
	stateArgPropPostWS: {
		// KDLv2: after seeing "ident WS", we don't yet know if ident is an arg or a property key.
		// If = follows, it's a property key. Otherwise, commit ident as an argument.
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// more whitespace, just ignore
			return nil
		},
		tokenizer.Equals: func(c *ParseContext, t tokenizer.Token) error {
			// It was a property key! Check that it's a valid identifier.
			if c.typeAnnot.Valid() {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}
			isIdentifier := false
			for _, class := range c.ident.ID.Classes() {
				if class == tokenizer.ClassIdentifier {
					isIdentifier = true
					break
				}
			}
			if !isIdentifier {
				return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
			}
			c.state = statePropertyValue
			return nil
		},
		tokenizer.ClassValue: func(c *ParseContext, t tokenizer.Token) error {
			// Another value means the original ident was an argument
			if c.ignoreNextArgProp {
				c.ignoreNextArgProp = false
			} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
				return err
			}
			c.typeAnnot.Clear()
			c.ident.Clear()

			// The new token becomes the current ident in stateArgProp
			c.ident = t
			c.state = stateArgProp
			return nil
		},
		tokenizer.ParensOpen: func(c *ParseContext, t tokenizer.Token) error {
			// Type annotation for the next value - commit ident as argument first
			if c.ignoreNextArgProp {
				c.ignoreNextArgProp = false
			} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
				return err
			}
			c.typeAnnot.Clear()
			c.ident.Clear()

			c.state = stateNodeParams
			c.pushState(stateTypeAnnot)
			return nil
		},
		tokenizer.BraceOpen: func(c *ParseContext, t tokenizer.Token) error {
			// Child block - commit ident as argument first
			if c.ident.Valid() {
				// KDLv2: entries cannot appear after a child block
				if c.childBlockSeen {
					return fmt.Errorf("entry found after child block")
				}
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}

			if c.ignoreNextArgProp || c.ignoreChildren > 0 {
				c.ignoreNextArgProp = false
				c.ignoreChildren++
			}

			c.childBlockSeen = true
			c.pushState(stateChildren)
			return nil
		},
		tokenizer.ClassTerminator: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				return nil
			}
			// Terminator - commit ident as argument
			if c.ident.Valid() {
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}

			_, _, err := c.popNodeAndState()
			return err
		},
		tokenizer.TokenComment: func(c *ParseContext, t tokenizer.Token) error {
			// Slashdash - commit ident as argument if present, then ignore next
			if c.ident.Valid() {
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}

			c.ignoreNextArgProp = true
			c.state = stateNodeParams
			return nil
		},
		tokenizer.SingleLineComment: func(c *ParseContext, t tokenizer.Token) error {
			if c.continuation {
				// Part of an escline - the comment is consumed along with the line
				return nil
			}
			// Single-line comment - commit ident as argument, node ends at EOL
			if c.ident.Valid() {
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}
			c.state = stateNodeEnd
			return nil
		},
		tokenizer.MultiLineComment: func(c *ParseContext, t tokenizer.Token) error {
			// Multi-line comment - just ignore, stay in this state
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.BraceClose: func(c *ParseContext, t tokenizer.Token) error {
			// Commit ident as argument
			if c.ident.Valid() {
				if c.ignoreNextArgProp {
					c.ignoreNextArgProp = false
				} else if err := c.currentNode().AddArgumentToken(c.ident, c.typeAnnot); err != nil {
					return err
				}
				c.typeAnnot.Clear()
				c.ident.Clear()
			}
			// End the current node
			_, _, err := c.popNodeAndState()
			if err != nil {
				return err
			}
			// Now handle the BraceClose in the parent state (should be stateChildren)
			if c.state == stateChildren {
				if c.ignoreChildren > 0 {
					c.ignoreChildren--
				}
				_, err = c.popState()
				return err
			}
			return fmt.Errorf("unexpected %s in state %s", t.ID, c.state)
		},
	},
	statePropertyValue: {
		tokenizer.Whitespace: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow whitespace after = and after type annotation before value
			return nil
		},
		tokenizer.MultiLineComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow inline comments between type annotation and value
			return nil
		},
		tokenizer.ClassComment: func(c *ParseContext, t tokenizer.Token) error {
			// KDLv2: allow comments in property value position
			return nil
		},
		tokenizer.Continuation: func(c *ParseContext, t tokenizer.Token) error {
			c.continuation = true
			return nil
		},
		tokenizer.ParensOpen: func(c *ParseContext, t tokenizer.Token) error {
			// a ( inside a node declaration is the beginning of a type annotation for a node
			c.pushState(stateTypeAnnot)
			return nil
		},
		tokenizer.ClassValue: func(c *ParseContext, t tokenizer.Token) error {
			if c.ignoreNextArgProp {
				c.ignoreNextArgProp = false
			} else if _, err := c.currentNode().AddPropertyToken(c.ident, t, c.typeAnnot); err != nil {
				return err
			}
			c.typeAnnot.Clear()
			c.ident.Clear()
			c.state = stateNode
			return nil
		},
	},
}
