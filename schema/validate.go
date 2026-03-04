package schema

import (
	"fmt"

	"github.com/stream-enterer/go2kdl/document"
)

// validator holds state during a validation run.
type validator struct {
	schema *schemaDoc
	errors []ValidationError
	path   []PathSegment
}

func (v *validator) pushPath(seg PathSegment) {
	v.path = append(v.path, seg)
}

func (v *validator) popPath() {
	v.path = v.path[:len(v.path)-1]
}

func (v *validator) addError(msg string, span document.Span, schemaID string) {
	pathCopy := make([]PathSegment, len(v.path))
	copy(pathCopy, v.path)
	v.errors = append(v.errors, ValidationError{
		Message:  msg,
		Path:     pathCopy,
		SchemaID: schemaID,
		Span:     span,
	})
}

func (v *validator) validateDocument(doc *document.Document) {
	v.validateNodeSet(
		doc.Nodes,
		v.schema.nodes,
		v.schema.nodeNames,
		v.schema.otherNodes,
		v.schema.tags,
		v.schema.otherTags,
	)
}

func (v *validator) validateNodeSet(
	nodes []*document.Node,
	defs []*nodeDef,
	nameConstraints *namesValidation,
	otherNodesAllowed bool,
	tagDefs []*tagDef,
	otherTagsAllowed bool,
) {
	// Find named and wildcard defs.
	var wildcard *nodeDef
	namedDefs := make(map[string]*nodeDef)
	for _, d := range defs {
		if d.name == "" {
			wildcard = d
		} else {
			namedDefs[d.name] = d
		}
	}

	// Count occurrences for cardinality.
	counts := make(map[string]int)
	for _, n := range nodes {
		name := nodeName(n)
		if _, exists := namedDefs[name]; exists {
			counts[name]++
		}
	}

	// Check cardinality.
	for _, d := range defs {
		if d.name == "" {
			continue
		}
		count := counts[d.name]
		if d.min != nil && count < *d.min {
			v.addError(
				fmt.Sprintf("expected at least %d %q node(s), found %d", *d.min, d.name, count),
				document.Span{}, d.id,
			)
		}
		if d.max != nil && count > *d.max {
			v.addError(
				fmt.Sprintf("expected at most %d %q node(s), found %d", *d.max, d.name, count),
				document.Span{}, d.id,
			)
		}
	}

	// Validate each node.
	for _, n := range nodes {
		name := nodeName(n)
		v.pushPath(PathSegment{Name: name, Kind: KindNode})

		// Validate node names.
		if nameConstraints != nil {
			v.validateNames(name, nameConstraints, n.Span)
		}

		named, hasNamed := namedDefs[name]
		matched := hasNamed || wildcard != nil

		if !matched {
			if !otherNodesAllowed {
				v.addError(
					fmt.Sprintf("node %q not allowed (not declared in schema)", name),
					n.Span, "",
				)
			}
			// If other-nodes-allowed, skip validation on this node entirely.
			v.popPath()
			continue
		}

		// Validate against named def.
		if hasNamed {
			v.validateNode(n, named)
		}

		// Validate against wildcard def (applies to ALL nodes including named).
		if wildcard != nil {
			v.validateNode(n, wildcard)
		}

		// Validate tags.
		if n.Type != "" {
			v.validateTag(n, tagDefs, otherTagsAllowed)
			// Also check node-level tag defs.
			var nodeLevelTagDefs []*tagDef
			if hasNamed {
				nodeLevelTagDefs = append(nodeLevelTagDefs, named.tags...)
			}
			if wildcard != nil {
				nodeLevelTagDefs = append(nodeLevelTagDefs, wildcard.tags...)
			}
			if len(nodeLevelTagDefs) > 0 {
				v.validateTag(n, nodeLevelTagDefs, otherTagsAllowed)
			}
		}

		v.popPath()
	}
}

func (v *validator) validateNode(n *document.Node, def *nodeDef) {
	// Validate properties.
	v.validateProperties(n, def)

	// Validate values (arguments).
	v.validateValues(n, def)

	// Validate children.
	v.validateChildren(n, def)
}

func (v *validator) validateProperties(n *document.Node, def *nodeDef) {
	// Build property def maps.
	var wildcardProp *propDef
	namedProps := make(map[string]*propDef)
	for _, pd := range def.props {
		if pd.key == "" {
			wildcardProp = pd
		} else {
			namedProps[pd.key] = pd
		}
	}

	// Check each property on the node.
	for _, key := range n.Properties.Keys() {
		val, _ := n.Properties.Get(key)
		named, hasNamed := namedProps[key]

		if !hasNamed && wildcardProp == nil && !def.otherProps {
			v.pushPath(PathSegment{Name: key, Kind: KindProperty})
			v.addError(
				fmt.Sprintf("property %q not allowed", key),
				val.Span, "",
			)
			v.popPath()
			continue
		}

		if hasNamed {
			v.pushPath(PathSegment{Name: key, Kind: KindProperty})
			for _, vl := range named.validations {
				if err := vl.validate(val); err != nil {
					v.addError(err.Error(), val.Span, named.id)
				}
			}
			v.popPath()
		}

		if wildcardProp != nil {
			v.pushPath(PathSegment{Name: key, Kind: KindProperty})
			for _, vl := range wildcardProp.validations {
				if err := vl.validate(val); err != nil {
					v.addError(err.Error(), val.Span, wildcardProp.id)
				}
			}
			v.popPath()
		}
	}

	// Check required properties.
	for _, pd := range def.props {
		if pd.required && pd.key != "" {
			if _, ok := n.Properties.Get(pd.key); !ok {
				v.addError(
					fmt.Sprintf("property %q is required but missing", pd.key),
					n.Span, pd.id,
				)
			}
		}
	}
}

func (v *validator) validateValues(n *document.Node, def *nodeDef) {
	if len(def.values) == 0 {
		return
	}

	argCount := len(n.Arguments)

	if len(def.values) == 1 {
		// Single valueDef applies to all arguments.
		vd := def.values[0]

		// Check count constraints.
		if vd.min != nil && argCount < *vd.min {
			v.addError(
				fmt.Sprintf("expected at least %d argument(s), got %d", *vd.min, argCount),
				n.Span, vd.id,
			)
		}
		if vd.max != nil && argCount > *vd.max {
			v.addError(
				fmt.Sprintf("expected at most %d argument(s), got %d", *vd.max, argCount),
				n.Span, vd.id,
			)
		}

		// Validate each argument.
		for i, arg := range n.Arguments {
			v.pushPath(PathSegment{Name: fmt.Sprintf("%d", i), Kind: KindArgument})
			for _, vl := range vd.validations {
				if err := vl.validate(arg); err != nil {
					v.addError(err.Error(), arg.Span, vd.id)
				}
			}
			v.popPath()
		}
	} else {
		// Multiple valueDefs: positional matching.
		for i, vd := range def.values {
			if i >= argCount {
				if vd.min != nil && *vd.min > 0 {
					v.addError(
						fmt.Sprintf("missing argument at position %d", i),
						n.Span, vd.id,
					)
				}
				continue
			}
			arg := n.Arguments[i]
			v.pushPath(PathSegment{Name: fmt.Sprintf("%d", i), Kind: KindArgument})
			for _, vl := range vd.validations {
				if err := vl.validate(arg); err != nil {
					v.addError(err.Error(), arg.Span, vd.id)
				}
			}
			v.popPath()
		}
	}
}

func (v *validator) validateChildren(n *document.Node, def *nodeDef) {
	hasChildren := len(n.Children) > 0

	if def.children == nil {
		// No children spec on the def.
		if hasChildren && def.name != "" {
			// Named def without children block: children not allowed.
			v.addError("node has children but schema does not allow children", n.Span, def.id)
		}
		// Wildcard with no children spec: children are unconstrained.
		return
	}

	if !hasChildren {
		return
	}

	// Recurse.
	v.validateNodeSet(
		n.Children,
		def.children.nodes,
		def.children.nodeNames,
		def.children.otherNodes,
		nil, // no tag defs at children level (tags are on the parent)
		false,
	)
}

func (v *validator) validateTag(n *document.Node, tagDefs []*tagDef, otherTagsAllowed bool) {
	if len(tagDefs) == 0 {
		return
	}

	tag := string(n.Type)
	if tag == "" {
		return
	}

	var wildcardTag *tagDef
	var matchedTag *tagDef
	for _, td := range tagDefs {
		if td.name == "" {
			wildcardTag = td
		} else if td.name == tag {
			matchedTag = td
		}
	}

	if matchedTag == nil && wildcardTag == nil {
		if !otherTagsAllowed {
			v.addError(
				fmt.Sprintf("type annotation (%s) not allowed", tag),
				n.Span, "",
			)
		}
		return
	}

	// Tag rules augment node rules — validate any additional node defs from the tag.
	if matchedTag != nil && len(matchedTag.nodes) > 0 {
		for _, nd := range matchedTag.nodes {
			v.validateNode(n, nd)
		}
	}
	if wildcardTag != nil && len(wildcardTag.nodes) > 0 {
		for _, nd := range wildcardTag.nodes {
			v.validateNode(n, nd)
		}
	}
}

func (v *validator) validateNames(name string, nv *namesValidation, span document.Span) {
	// Wrap the name in a temporary Value for validation.
	tmpVal := &document.Value{Value: name}
	for _, vl := range nv.validations {
		if err := vl.validate(tmpVal); err != nil {
			v.addError(
				fmt.Sprintf("node name %q: %s", name, err.Error()),
				span, "",
			)
		}
	}
}
