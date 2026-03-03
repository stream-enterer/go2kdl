//go:build kdlunordered

package kdl

// kdlOutputMarshalKDLNodeExpected is the expected output for the marshalKDLNode test.
// With unordered properties, map iteration is sorted alphabetically: active then age.
const kdlOutputMarshalKDLNodeExpected = `
father "BOB" "JOHNSON" active=#true age=32
mother "JANE" "JOHNSON" active=#true age=28
`
