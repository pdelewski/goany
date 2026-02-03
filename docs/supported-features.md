# Supported Features

This document details the Go language features supported by goany for cross-platform transpilation.

## Types

### Primitive Types
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `float32`, `float64`
- `bool`
- `string`

### Composite Types
- **Slices**: `[]T` - dynamic arrays
- **Structs**: custom types with fields
- **Function types**: functions as first-class values

## Language Constructs

### Variables
- Variable declarations: `var x int`
- Short declarations: `x := 10`
- Multiple assignments: `a, b := 1, 2`

### Functions
- Regular functions with parameters and return values
- Multiple return values: `func foo() (int, error)`
- Methods on structs: `func (s *MyStruct) Method()`

### Control Flow
- `if`/`else` statements
- `for` loops (C-style and range-based)
- `switch` statements
- `break` and `continue`

### Operators
- Arithmetic: `+`, `-`, `*`, `/`, `%`
- Comparison: `==`, `!=`, `<`, `>`, `<=`, `>=`
- Logical: `&&`, `||`, `!`
- Bitwise: `&`, `|`, `^`, `<<`, `>>`

## Limitations

Some Go features are not supported due to differences in target platforms:

### Not Supported
- Goroutines and channels (concurrency)
- Interfaces (partial support)
- Pointers (limited support)
- Reflection
- `defer` statements
- `panic`/`recover`

### Maps (Partial Support)

Maps are supported with the following limitations:

**Supported:**
- `make(map[K]V)` - map creation
- `m[key] = value` - map set
- `value := m[key]` - map get
- `delete(m, key)` - map delete
- `len(m)` - map length
- `value, ok := m[key]` - comma-ok idiom
- Maps as struct fields
- Maps as function parameters
- Maps as function return values
- Key types: `string`, `int`, `int8`-`int64`, `uint8`-`uint64`, `float32`, `float64`, `bool`

**Not Supported:**
- Nested maps (`map[string]map[string]int`)
- Map literals (`map[string]int{"a": 1, "b": 2}`)
- Range over maps (`for k, v := range m`)
- Structs as map keys
- Map comparison

**Known Issues:**
- Assigning to a nil map doesn't panic (undefined behavior)
- Rust debug builds may overflow in string hash function (use release builds)

### Backend-Specific Notes

See [rust_backend_rules.md](../cmd/doc/rust_backend_rules.md) for detailed Rust backend implementation notes.

## Known Issues

### C# Slice Syntax

The slice expression `arr[:n]` may not emit correctly in C#. Use a manual loop instead:

```go
// Instead of: arr = arr[:n]
// Use:
newArr := []T{}
i := 0
for {
    if i >= n { break }
    newArr = append(newArr, arr[i])
    i = i + 1
}
arr = newArr
```

## Graphics Runtime

A cross-platform 2D graphics library for window creation and drawing shapes.

### Native Backends (C++, C#, Rust)

Two backends are available, selected via `-graphics-runtime` flag:

| Backend | Platforms | Dependencies | Notes |
|---------|-----------|--------------|-------|
| `tigr` | C++, C#, Rust | C compiler (bundled source) | Default, lightweight |
| `sdl2` | C++, C#, Rust | SDL2 library | Hardware accelerated, full-featured |

### JavaScript Backend

Uses the HTML5 Canvas API and runs directly in the browser - no external dependencies required.

### Setup SDL2 Dependencies

Only needed for `-graphics-runtime=sdl2`:

```bash
./scripts/setup-deps.sh
```

### Go Execution

When running Go code directly (before transpilation):

| Backend | Build Command | Dependencies | Notes |
|---------|---------------|--------------|-------|
| `tigr` | `go build` | C compiler | Default, no external libs |
| `sdl2` | `go build -tags sdl2` | SDL2 library | Hardware accelerated |

```bash
# Default: tigr backend (no SDL2 dependency)
cd examples/mos6502/cmd/c64
go run .

# SDL2 backend (requires SDL2 installed)
go build -tags sdl2
./c64
```

### Example Usage

```go
package main

import "myapp/graphics"

func main() {
    w := graphics.CreateWindow("Demo", 800, 600)

    running := true
    for running {
        w, running = graphics.PollEvents(w)
        graphics.Clear(w, graphics.Black())
        graphics.FillRect(w, graphics.NewRect(100, 100, 200, 150), graphics.Red())
        graphics.Present(w)
    }

    graphics.CloseWindow(w)
}
```

See [runtime/graphics/README.md](../runtime/graphics/README.md) for full API documentation.
