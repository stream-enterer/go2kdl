package schema

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	kdl "github.com/stream-enterer/go2kdl"
	"github.com/stream-enterer/go2kdl/document"
)

// parseKDL parses KDL input using default options.
func parseKDL(r io.Reader) (*document.Document, error) {
	return kdl.Parse(r)
}

// compile transforms a parsed KDL document into an internal schema model.
func compile(doc *document.Document) (*schemaDoc, error) {
	// Find exactly one top-level "document" node.
	var docNode *document.Node
	for _, n := range doc.Nodes {
		name := nodeName(n)
		if name == "document" {
			if docNode != nil {
				return nil, fmt.Errorf("schema must have exactly one top-level 'document' node, found multiple")
			}
			docNode = n
		} else {
			return nil, fmt.Errorf("unexpected top-level node %q; only 'document' is allowed", name)
		}
	}
	if docNode == nil {
		return nil, fmt.Errorf("schema must have a top-level 'document' node")
	}

	sd := &schemaDoc{}

	for _, child := range docNode.Children {
		name := nodeName(child)
		switch name {
		case "info":
			info, err := compileInfo(child)
			if err != nil {
				return nil, err
			}
			sd.info = info
		case "node":
			nd, err := compileNode(child)
			if err != nil {
				return nil, err
			}
			sd.nodes = append(sd.nodes, nd)
		case "tag":
			td, err := compileTag(child)
			if err != nil {
				return nil, err
			}
			sd.tags = append(sd.tags, td)
		case "definitions":
			defs, err := compileDefinitions(child)
			if err != nil {
				return nil, err
			}
			sd.defs = defs
		case "node-names":
			nv, err := compileNamesValidation(child)
			if err != nil {
				return nil, err
			}
			sd.nodeNames = nv
		case "other-nodes-allowed":
			sd.otherNodes = boolArg(child)
		case "other-tags-allowed":
			sd.otherTags = boolArg(child)
		}
	}

	// Resolve refs.
	if err := resolveRefs(sd); err != nil {
		return nil, err
	}

	return sd, nil
}

func compileNode(n *document.Node) (*nodeDef, error) {
	nd := &nodeDef{
		name: firstArgString(n),
		id:   propString(n, "id"),
		ref:  propString(n, "ref"),
	}

	if v := propString(n, "description"); v != "" {
		nd.description = v
	}

	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "min":
			nd.min = intArg(child)
		case "max":
			nd.max = intArg(child)
		case "prop":
			pd, err := compileProp(child)
			if err != nil {
				return nil, err
			}
			nd.props = append(nd.props, pd)
		case "value":
			vd, err := compileValue(child)
			if err != nil {
				return nil, err
			}
			nd.values = append(nd.values, vd)
		case "children":
			cd, err := compileChildren(child)
			if err != nil {
				return nil, err
			}
			nd.children = cd
		case "tag":
			td, err := compileTag(child)
			if err != nil {
				return nil, err
			}
			nd.tags = append(nd.tags, td)
		case "prop-names":
			nv, err := compileNamesValidation(child)
			if err != nil {
				return nil, err
			}
			nd.propNames = nv
		case "other-props-allowed":
			nd.otherProps = boolArg(child)
		}
	}

	return nd, nil
}

func compileProp(n *document.Node) (*propDef, error) {
	pd := &propDef{
		key: firstArgString(n),
		id:  propString(n, "id"),
		ref: propString(n, "ref"),
	}

	if v := propString(n, "description"); v != "" {
		pd.description = v
	}

	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "required":
			pd.required = boolArg(child)
		default:
			v, err := compileValidation(child)
			if err != nil {
				return nil, err
			}
			if v != nil {
				pd.validations = append(pd.validations, v)
			}
		}
	}

	return pd, nil
}

func compileValue(n *document.Node) (*valueDef, error) {
	vd := &valueDef{
		id:  propString(n, "id"),
		ref: propString(n, "ref"),
	}

	if v := propString(n, "description"); v != "" {
		vd.description = v
	}

	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "min":
			vd.min = intArg(child)
		case "max":
			vd.max = intArg(child)
		default:
			v, err := compileValidation(child)
			if err != nil {
				return nil, err
			}
			if v != nil {
				vd.validations = append(vd.validations, v)
			}
		}
	}

	return vd, nil
}

func compileChildren(n *document.Node) (*childrenDef, error) {
	cd := &childrenDef{
		id:  propString(n, "id"),
		ref: propString(n, "ref"),
	}

	if v := propString(n, "description"); v != "" {
		cd.description = v
	}

	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "node":
			nd, err := compileNode(child)
			if err != nil {
				return nil, err
			}
			cd.nodes = append(cd.nodes, nd)
		case "node-names":
			nv, err := compileNamesValidation(child)
			if err != nil {
				return nil, err
			}
			cd.nodeNames = nv
		case "other-nodes-allowed":
			cd.otherNodes = boolArg(child)
		}
	}

	return cd, nil
}

func compileTag(n *document.Node) (*tagDef, error) {
	td := &tagDef{
		name: firstArgString(n),
		id:   propString(n, "id"),
		ref:  propString(n, "ref"),
	}

	if v := propString(n, "description"); v != "" {
		td.description = v
	}

	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "node":
			nd, err := compileNode(child)
			if err != nil {
				return nil, err
			}
			td.nodes = append(td.nodes, nd)
		case "node-names":
			nv, err := compileNamesValidation(child)
			if err != nil {
				return nil, err
			}
			td.nodeNames = nv
		case "other-nodes-allowed":
			td.otherNodes = boolArg(child)
		}
	}

	return td, nil
}

func compileDefinitions(n *document.Node) (*definitions, error) {
	d := &definitions{}
	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "node":
			nd, err := compileNode(child)
			if err != nil {
				return nil, err
			}
			d.nodes = append(d.nodes, nd)
		case "tag":
			td, err := compileTag(child)
			if err != nil {
				return nil, err
			}
			d.tags = append(d.tags, td)
		case "prop":
			pd, err := compileProp(child)
			if err != nil {
				return nil, err
			}
			d.props = append(d.props, pd)
		case "value":
			vd, err := compileValue(child)
			if err != nil {
				return nil, err
			}
			d.values = append(d.values, vd)
		case "children":
			cd, err := compileChildren(child)
			if err != nil {
				return nil, err
			}
			d.children = append(d.children, cd)
		}
	}
	return d, nil
}

func compileNamesValidation(n *document.Node) (*namesValidation, error) {
	nv := &namesValidation{}
	for _, child := range n.Children {
		v, err := compileValidation(child)
		if err != nil {
			return nil, err
		}
		if v != nil {
			nv.validations = append(nv.validations, v)
		}
	}
	return nv, nil
}

func compileInfo(n *document.Node) (*schemaInfo, error) {
	info := &schemaInfo{}
	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "title":
			info.title = firstArgString(child)
		case "description":
			info.description = firstArgString(child)
		case "author":
			info.author = firstArgString(child)
		case "license":
			info.license = firstArgString(child)
		case "link":
			info.link = firstArgString(child)
		}
	}
	return info, nil
}

// compileValidation dispatches to the appropriate validation type.
func compileValidation(n *document.Node) (validation, error) {
	name := nodeName(n)
	switch name {
	case "type":
		return &typeValidation{typeName: firstArgString(n)}, nil
	case "enum":
		var allowed []any
		for _, arg := range n.Arguments {
			allowed = append(allowed, arg.Value)
		}
		return &enumValidation{allowed: allowed}, nil
	case "pattern":
		s := firstArgString(n)
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", s, err)
		}
		return &patternValidation{pattern: re}, nil
	case "min-length":
		v := intArg(n)
		return &stringLengthValidation{min: v}, nil
	case "max-length":
		v := intArg(n)
		return &stringLengthValidation{max: v}, nil
	case "format":
		return &formatValidation{format: firstArgString(n)}, nil
	case ">", ">=", "<", "<=":
		f := floatArg(n)
		return &numberRangeValidation{op: name, value: f}, nil
	case "%":
		f := floatArg(n)
		return &numberMultipleValidation{divisor: f}, nil
	default:
		// Unknown validation nodes are silently ignored for forward-compatibility.
		return nil, nil
	}
}

// Helper functions for extracting values from nodes.

func nodeName(n *document.Node) string {
	return n.Name.ValueString()
}

func firstArgString(n *document.Node) string {
	if len(n.Arguments) == 0 {
		return ""
	}
	if s, ok := n.Arguments[0].Value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", n.Arguments[0].Value)
}

func boolArg(n *document.Node) bool {
	if len(n.Arguments) == 0 {
		return false
	}
	if b, ok := n.Arguments[0].Value.(bool); ok {
		return b
	}
	return false
}

func intArg(n *document.Node) *int {
	if len(n.Arguments) == 0 {
		return nil
	}
	switch v := n.Arguments[0].Value.(type) {
	case int64:
		i := int(v)
		return &i
	case float64:
		i := int(v)
		return &i
	}
	return nil
}

func floatArg(n *document.Node) float64 {
	if len(n.Arguments) == 0 {
		return 0
	}
	switch v := n.Arguments[0].Value.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

func propString(n *document.Node, key string) string {
	v, ok := n.Properties.Get(key)
	if !ok {
		return ""
	}
	if s, ok := v.Value.(string); ok {
		return s
	}
	return ""
}

// --- Ref resolution ---

func resolveRefs(sd *schemaDoc) error {
	visited := make(map[string]bool)

	for _, nd := range sd.nodes {
		if err := resolveNodeRef(nd, sd.defs, visited); err != nil {
			return err
		}
	}
	for _, td := range sd.tags {
		if err := resolveTagRef(td, sd.defs, visited); err != nil {
			return err
		}
	}
	return nil
}

func resolveNodeRef(nd *nodeDef, defs *definitions, visited map[string]bool) error {
	if nd.ref != "" {
		if visited[nd.ref] {
			return fmt.Errorf("circular ref detected: %s", nd.ref)
		}
		visited[nd.ref] = true
		defer delete(visited, nd.ref)

		resolved, err := findNodeDef(nd.ref, defs)
		if err != nil {
			return err
		}
		// Resolve the referenced def's own refs first.
		if err := resolveNodeRef(resolved, defs, visited); err != nil {
			return err
		}
		mergeNodeDef(nd, resolved)
	}

	// Recurse into children.
	for _, pd := range nd.props {
		if err := resolvePropRef(pd, defs, visited); err != nil {
			return err
		}
	}
	for _, vd := range nd.values {
		if err := resolveValueRef(vd, defs, visited); err != nil {
			return err
		}
	}
	if nd.children != nil {
		if err := resolveChildrenRef(nd.children, defs, visited); err != nil {
			return err
		}
	}
	for _, td := range nd.tags {
		if err := resolveTagRef(td, defs, visited); err != nil {
			return err
		}
	}
	return nil
}

func resolvePropRef(pd *propDef, defs *definitions, visited map[string]bool) error {
	if pd.ref == "" || defs == nil {
		return nil
	}
	if visited[pd.ref] {
		return fmt.Errorf("circular ref detected: %s", pd.ref)
	}
	visited[pd.ref] = true
	defer delete(visited, pd.ref)

	resolved, err := findPropDef(pd.ref, defs)
	if err != nil {
		return err
	}
	if err := resolvePropRef(resolved, defs, visited); err != nil {
		return err
	}
	mergePropDef(pd, resolved)
	return nil
}

func resolveValueRef(vd *valueDef, defs *definitions, visited map[string]bool) error {
	if vd.ref == "" || defs == nil {
		return nil
	}
	if visited[vd.ref] {
		return fmt.Errorf("circular ref detected: %s", vd.ref)
	}
	visited[vd.ref] = true
	defer delete(visited, vd.ref)

	resolved, err := findValueDef(vd.ref, defs)
	if err != nil {
		return err
	}
	if err := resolveValueRef(resolved, defs, visited); err != nil {
		return err
	}
	mergeValueDef(vd, resolved)
	return nil
}

func resolveChildrenRef(cd *childrenDef, defs *definitions, visited map[string]bool) error {
	if cd.ref != "" {
		if defs == nil {
			return fmt.Errorf("ref %q but no definitions block", cd.ref)
		}
		if visited[cd.ref] {
			return fmt.Errorf("circular ref detected: %s", cd.ref)
		}
		visited[cd.ref] = true
		defer delete(visited, cd.ref)

		resolved, err := findChildrenDef(cd.ref, defs)
		if err != nil {
			return err
		}
		if err := resolveChildrenRef(resolved, defs, visited); err != nil {
			return err
		}
		mergeChildrenDef(cd, resolved)
	}

	for _, nd := range cd.nodes {
		if err := resolveNodeRef(nd, defs, visited); err != nil {
			return err
		}
	}
	return nil
}

func resolveTagRef(td *tagDef, defs *definitions, visited map[string]bool) error {
	if td.ref == "" || defs == nil {
		return nil
	}
	if visited[td.ref] {
		return fmt.Errorf("circular ref detected: %s", td.ref)
	}
	visited[td.ref] = true
	defer delete(visited, td.ref)

	resolved, err := findTagDef(td.ref, defs)
	if err != nil {
		return err
	}
	if err := resolveTagRef(resolved, defs, visited); err != nil {
		return err
	}
	mergeTagDef(td, resolved)

	for _, nd := range td.nodes {
		if err := resolveNodeRef(nd, defs, visited); err != nil {
			return err
		}
	}
	return nil
}

// parseRef parses a ref string into kind and target.
// Supports: [id="x"] and definitions > name
func parseRef(ref string) (kind string, target string, err error) {
	ref = strings.TrimSpace(ref)

	// [id="x"] pattern
	if strings.HasPrefix(ref, "[id=") {
		// Extract the value from [id="x"] or [id='x']
		inner := ref[4 : len(ref)-1] // strip [id= and ]
		inner = strings.Trim(inner, `"'`)
		return "id", inner, nil
	}

	// definitions > name pattern
	if strings.Contains(ref, ">") {
		parts := strings.Split(ref, ">")
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if left == "definitions" {
				return "name", right, nil
			}
		}
	}

	return "", "", fmt.Errorf("unsupported ref pattern: %q (only [id=\"x\"] and \"definitions > name\" are supported)", ref)
}

func findNodeDef(ref string, defs *definitions) (*nodeDef, error) {
	if defs == nil {
		return nil, fmt.Errorf("ref %q but no definitions block", ref)
	}
	kind, target, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	for _, nd := range defs.nodes {
		if kind == "id" && nd.id == target {
			return nd, nil
		}
		if kind == "name" && nd.name == target {
			return nd, nil
		}
	}
	return nil, fmt.Errorf("ref %q: no matching node definition found", ref)
}

func findPropDef(ref string, defs *definitions) (*propDef, error) {
	if defs == nil {
		return nil, fmt.Errorf("ref %q but no definitions block", ref)
	}
	kind, target, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	for _, pd := range defs.props {
		if kind == "id" && pd.id == target {
			return pd, nil
		}
		if kind == "name" && pd.key == target {
			return pd, nil
		}
	}
	return nil, fmt.Errorf("ref %q: no matching prop definition found", ref)
}

func findValueDef(ref string, defs *definitions) (*valueDef, error) {
	if defs == nil {
		return nil, fmt.Errorf("ref %q but no definitions block", ref)
	}
	kind, target, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	for _, vd := range defs.values {
		if kind == "id" && vd.id == target {
			return vd, nil
		}
	}
	// For value defs, name-based lookup searches by id since values don't have names.
	return nil, fmt.Errorf("ref %q: no matching value definition found", ref)
}

func findChildrenDef(ref string, defs *definitions) (*childrenDef, error) {
	if defs == nil {
		return nil, fmt.Errorf("ref %q but no definitions block", ref)
	}
	kind, target, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	for _, cd := range defs.children {
		if kind == "id" && cd.id == target {
			return cd, nil
		}
	}
	return nil, fmt.Errorf("ref %q: no matching children definition found", ref)
}

func findTagDef(ref string, defs *definitions) (*tagDef, error) {
	if defs == nil {
		return nil, fmt.Errorf("ref %q but no definitions block", ref)
	}
	kind, target, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	for _, td := range defs.tags {
		if kind == "id" && td.id == target {
			return td, nil
		}
		if kind == "name" && td.name == target {
			return td, nil
		}
	}
	return nil, fmt.Errorf("ref %q: no matching tag definition found", ref)
}

// Merge functions: local declarations take precedence.

func mergeNodeDef(dst, src *nodeDef) {
	if dst.description == "" {
		dst.description = src.description
	}
	dst.props = append(dst.props, src.props...)
	dst.values = append(dst.values, src.values...)
	if dst.children == nil {
		dst.children = src.children
	}
	dst.tags = append(dst.tags, src.tags...)
	if dst.propNames == nil {
		dst.propNames = src.propNames
	}
	if dst.min == nil {
		dst.min = src.min
	}
	if dst.max == nil {
		dst.max = src.max
	}
}

func mergePropDef(dst, src *propDef) {
	if dst.description == "" {
		dst.description = src.description
	}
	dst.validations = append(dst.validations, src.validations...)
}

func mergeValueDef(dst, src *valueDef) {
	if dst.description == "" {
		dst.description = src.description
	}
	dst.validations = append(dst.validations, src.validations...)
	if dst.min == nil {
		dst.min = src.min
	}
	if dst.max == nil {
		dst.max = src.max
	}
}

func mergeChildrenDef(dst, src *childrenDef) {
	if dst.description == "" {
		dst.description = src.description
	}
	dst.nodes = append(dst.nodes, src.nodes...)
	if dst.nodeNames == nil {
		dst.nodeNames = src.nodeNames
	}
}

func mergeTagDef(dst, src *tagDef) {
	if dst.description == "" {
		dst.description = src.description
	}
	dst.nodes = append(dst.nodes, src.nodes...)
	if dst.nodeNames == nil {
		dst.nodeNames = src.nodeNames
	}
}

