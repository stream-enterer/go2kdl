package schema

// schemaDoc is the root of a compiled schema.
type schemaDoc struct {
	info       *schemaInfo
	nodes      []*nodeDef
	tags       []*tagDef
	defs       *definitions
	nodeNames  *namesValidation
	otherNodes bool
	otherTags  bool
}

// nodeDef describes a node that may appear at a given level.
type nodeDef struct {
	name        string // empty = wildcard (matches any node)
	description string
	id          string
	ref         string
	min         *int
	max         *int
	props       []*propDef
	values      []*valueDef
	children    *childrenDef
	tags        []*tagDef
	propNames   *namesValidation
	otherProps  bool
}

// tagDef describes a tag constraint.
type tagDef struct {
	name        string // empty = wildcard
	description string
	id          string
	ref         string
	nodes       []*nodeDef
	nodeNames   *namesValidation
	otherNodes  bool
}

// propDef describes a property constraint.
type propDef struct {
	key         string // empty = wildcard
	description string
	id          string
	ref         string
	required    bool
	validations []validation
}

// valueDef describes a value (argument) constraint.
type valueDef struct {
	description string
	id          string
	ref         string
	min         *int
	max         *int
	validations []validation
}

// childrenDef describes the allowed children block.
type childrenDef struct {
	description string
	id          string
	ref         string
	nodes       []*nodeDef
	nodeNames   *namesValidation
	otherNodes  bool
}

// definitions holds reusable schema fragments.
type definitions struct {
	nodes    []*nodeDef
	tags     []*tagDef
	props    []*propDef
	values   []*valueDef
	children []*childrenDef
}

// namesValidation constrains allowed names (node-names or prop-names).
type namesValidation struct {
	validations []validation
}

// schemaInfo holds optional metadata about the schema.
type schemaInfo struct {
	title       string
	description string
	author      string
	license     string
	link        string
}
