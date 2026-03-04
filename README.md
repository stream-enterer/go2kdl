# go2kdl

A Go library for [KDL v2](https://kdl.dev/) with full spec compliance, format-preserving editing, and rich error diagnostics.

Forked from [sblinch/kdl-go](https://github.com/sblinch/kdl-go) and upgraded to KDLv2 from the ground up.

## Features

- **Full KDLv2 spec compliance** — passes all 336 official test cases
- **Format-preserving editing** — parse a config, change one value, write it back without disturbing whitespace, comments, or formatting
- **Rich error diagnostics** — structured errors with line, column, byte offset, and source context snippets
- **Familiar API** — `Marshal`/`Unmarshal` with struct tags, similar to `encoding/json`
- **Custom (un)marshalers** — `Marshaler`/`Unmarshaler` interfaces plus generic `AddCustomMarshaler[T]`/`AddCustomUnmarshaler[T]`
- **Format options** — `time.Time`, `time.Duration`, `[]byte`, `float32/64` with `format` tag support
- **Streaming** — `Encoder`/`Decoder` for reading and writing KDL from `io.Reader`/`io.Writer`
- **Comment preservation** — parse and round-trip comments
- **Relaxed parsing modes** — nginx-style configs, YAML/TOML-style assignments, multiplier suffixes
- **Node lookup helpers** — `FindNode`, `FindNodeRecursive`, `RemoveNode` on both `Document` and `Node`

## Install

```
go get github.com/stream-enterer/go2kdl
```

## Import

```go
import kdl "github.com/stream-enterer/go2kdl"
```

## Quick start

### Parse and generate

```go
data := `
name "Bob"
age 76
active #true
`

doc, err := kdl.Parse(strings.NewReader(data))
if err != nil {
    log.Fatal(kdl.FormatError(err))
}

for _, node := range doc.Nodes {
    fmt.Println(node.Name.ValueString())
}
// name
// age
// active
```

### Unmarshal into a struct

```go
type Person struct {
    Name   string `kdl:"name"`
    Age    int    `kdl:"age"`
    Active bool   `kdl:"active"`
}

data := []byte(`
name "Bob"
age 76
active #true
`)

var p Person
if err := kdl.Unmarshal(data, &p); err != nil {
    log.Fatal(err)
}
fmt.Printf("%+v\n", p)
// {Name:Bob Age:76 Active:true}
```

### Marshal from a struct

```go
person := Person{Name: "Bob", Age: 32, Active: true}

data, err := kdl.Marshal(person)
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(data))
// name "Bob"
// age 32
// active #true
```

## Format-preserving editing

Parse a document, modify it, and write it back. Untouched nodes are emitted
byte-for-byte identical to the original source.

```go
input := `
// Database settings
host "localhost"
port 5432
max-connections 100
`

doc, _ := kdl.Parse(strings.NewReader(input))

// Change one value
node := doc.FindNode("port")
node.Arguments[0].SetValue(int64(3306))

// Write it back — only the changed value differs
kdl.GenerateWithOptions(doc, os.Stdout, kdl.GenerateOptions{
    PreserveFormatting: true,
})
```

The setter methods (`SetValue`, `SetType`, `SetName`, `AddArgument`, etc.)
automatically mark elements as dirty so the generator knows to re-format them.
Everything else is emitted verbatim from the original source.

Use `Autoformat` when you want to reformat an entire document:

```go
kdl.Autoformat(reader, writer)
```

## Error diagnostics

Parse errors include source location and context:

```go
_, err := kdl.Parse(strings.NewReader(`node {`))
if err != nil {
    fmt.Println(kdl.FormatError(err))
}
```

```
error: unexpected EOF in state stateChildren
 --> 1:7
 |
 1 | node {
 |       ^
```

Errors implement `error` and can be inspected programmatically:

```go
var kdlErr *kdl.Error
if errors.As(err, &kdlErr) {
    fmt.Println(kdlErr.Span.Line, kdlErr.Span.Column)
}
```

## Decoder with options

```go
var person Person
dec := kdl.NewDecoder(strings.NewReader(data))
dec.Options.AllowUnhandledNodes = true
if err := dec.Decode(&person); err != nil {
    log.Fatal(err)
}
```

## Encoder with options

```go
enc := kdl.NewEncoder(os.Stdout)
if err := enc.Encode(person); err != nil {
    log.Fatal(err)
}
```

## Relaxed parsing modes

### nginx-style syntax

```go
data := `
location / {
    root /var/www/html;
}
location /missing {
    return 404;
}
`

type Location struct {
    Root   string `kdl:"root,omitempty,child"`
    Return int    `kdl:"return,omitempty,child"`
}
type Server struct {
    Locations map[string]Location `kdl:"location,multiple"`
}

var srv Server
dec := kdl.NewDecoder(strings.NewReader(data))
dec.Options.RelaxedNonCompliant |= relaxed.NGINXSyntax
dec.Decode(&srv)
```

## Document model

The AST is fully accessible for programmatic manipulation:

```go
doc := document.New()

node := document.NewNode()
node.SetName("server")
node.AddArgument("localhost", "")
node.AddProperty("port", int64(8080), "")
doc.AddNode(node)

// Find nodes
n := doc.FindNode("server")
n = doc.FindNodeRecursive("nested-node")

// Remove nodes
doc.Nodes[0].RemoveNode("child-name")
```

## Spec compliance

go2kdl passes all 336 official KDLv2 test cases from the
[kdl-org test suite](https://github.com/kdl-org/kdl/tree/main/tests/test_cases):

```
go test -run TestKDLv2Compliance -v
```

```
=== RUN   TestKDLv2Compliance
--- PASS: TestKDLv2Compliance (0.01s)
    336/336 tests passed
```

## KDLv2 syntax at a glance

For users coming from KDLv1, the main changes are:

| KDLv1 | KDLv2 |
|---|---|
| `true` / `false` / `null` | `#true` / `#false` / `#null` |
| `r"raw string"` | `#"raw string"#` |
| N/A | `#inf` / `#-inf` / `#nan` |
| N/A | `"""multiline"""` |
| N/A | Bare identifiers as values |
| N/A | Whitespace in type annotations: `( u8 )` |
| N/A | Space around `=`: `key = value` |

See the [KDLv2 spec](https://kdl.dev/) for the full details.

## License

go2kdl is released under the MIT license. See [LICENSE](LICENSE) for details.
