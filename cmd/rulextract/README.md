# rulextract

A tool that extracts structural signatures of Go AST constructs from tests and examples, providing visibility into which language construct variants are exercised.

## Purpose

GoAny transpiles a subset of Go to C++, C#, Rust, and JavaScript. Not every Go construct is supported, and supported constructs have specific variants (e.g., different for-loop patterns, different assignment forms). `rulextract` answers the question: **what construct variants are actually covered by our tests and examples?**

This is a coverage visibility tool, not an enforcement tool. It helps identify gaps — if a construct variant isn't listed, it either isn't supported or isn't tested.

## Usage

```
go build -o rulextract ./cmd/rulextract/
./rulextract [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-format` | `text` | Output format: `text`, `json`, `markdown` |
| `-root` | auto-detect | Project root directory (looks for `tests/` directory) |

### Examples

```bash
# Human-readable output
./rulextract

# JSON for tooling
./rulextract -format json > patterns.json

# Markdown for documentation
./rulextract -format markdown > supported-constructs.md

# Explicit project root
./rulextract -root /path/to/project
```

## What It Extracts

The tool walks all `.go` files (excluding `_test.go`) in `tests/` and `examples/` directories and extracts structural signatures for 23 AST node categories:

| Category | What it captures |
|----------|-----------------|
| AssignStmt | Assignment variants: `ident := int`, `ident = call()`, `expr[expr] = ident`, ... |
| BinaryExpr | Binary operators: `expr + expr`, `expr == expr`, `expr && expr`, ... |
| BranchStmt | `break`, `continue` |
| CallExpr | Call patterns: `ident(ident, int)`, `ident.field(expr)`, ... |
| CompositeLit | Literal styles: `[]ident{positional}`, `ident{keyed}`, `map[ident]ident{empty}`, ... |
| ConstDecl | Constant declarations with type/value patterns |
| ForStmt | For-loop variants: `for ident := int; ident < int; ident++`, ... |
| FuncDecl | Function signatures with receiver, params, returns |
| FuncLit | Anonymous function signatures |
| IfStmt | If variants with init, condition style, else/else-if |
| IncDecStmt | `ident++`, `ident--`, `ident.field++` |
| IndexExpr | Index access patterns: `ident[ident]`, `ident.field[expr]`, ... |
| InterfaceType | `interface{}`, `interface{...}` |
| MapType | Map type declarations: `map[ident]ident`, `map[ident][]ident`, ... |
| RangeStmt | Range variants: `for ident, ident := range ident`, ... |
| ReturnStmt | Return patterns: `return`, `return ident`, `return ident, ident`, ... |
| SliceExpr | Slice operations: `ident[int:int]`, `ident[:ident]`, ... |
| SliceType | Slice type declarations: `[]ident`, `[][]ident`, ... |
| SwitchStmt | Switch variants with/without tag and init |
| TypeAssert | Type assertions: `ident.(ident)`, `ident.(type)` |
| TypeDecl | Type declarations: `type ident struct{...}`, `type ident ident`, ... |
| UnaryExpr | Unary operators: `&ident`, `-ident`, `&ident.field` |
| VarDecl | Variable declarations with type/value patterns |

## Signature Abstraction

Signatures abstract away concrete values while preserving structural shape. Three abstraction levels are used depending on context:

**Full signature** — used for loop init/cond/post, where the exact structure matters:
```
for ident := int; ident < int; ident++
for ident := int; ident < ident(ident); ident++
```

**Shallow signature** — used for if-conditions and LHS of assignments, one level deep:
```
if expr == expr [no-else]
if ident := call(); expr != expr [else]
```

**Argument-level signature** — used for call arguments and assignment RHS, coarsest level:
```
ident := call()
ident := ident.field
ident = expr
```

Concrete names are replaced (`ident`), literals by type (`int`, `string`, `float`), field access is generic (`.field`), complex sub-expressions collapse to `expr`.

## Output Formats

### Text (default)

```
=== ForStmt ===
  for ident := int; ident < int; ident++
    -> tests/lang-constructs/main.go:88
       for x := 0; x < 10; x++ {
    -> tests/lang-constructs/main.go:182
       for i := 1; i <= 5; i++ {
    -> ... and 4 more
```

Each signature shows up to 3 locations with source code. Additional locations are summarized.

### JSON

```json
[
  {
    "category": "ForStmt",
    "signature": "for ident := int; ident < int; ident++",
    "count": 6,
    "locations": [
      {
        "location": "tests/lang-constructs/main.go:88",
        "source": "for x := 0; x < 10; x++ {"
      }
    ]
  }
]
```

### Markdown

Produces a table per category with signature, occurrence count, example location, and source code.

## Scanned Directories

The tool scans:
- `tests/` — language construct tests
- `examples/` — example programs

It skips hidden directories, `vendor/`, `build/`, `node_modules/`, and `_test.go` files.
