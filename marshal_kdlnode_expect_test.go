//go:build !kdlunordered

package kdl

// kdlOutputMarshalKDLNodeExpected is the expected output for the marshalKDLNode test.
// With ordered properties (default), insertion order is preserved: age then active.
const kdlOutputMarshalKDLNodeExpected = `
father "BOB" "JOHNSON" age=32 active=#true
mother "JANE" "JOHNSON" age=28 active=#true
`
