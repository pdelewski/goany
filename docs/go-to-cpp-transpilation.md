# Go to C++ Transpilation Rules

This document describes the baseline rules the ULang transpiler uses to convert Go source code into C++. It covers type mappings, control flow, expressions, declarations, copy semantics, and built-in functions.

---

## 1. Primitive Type Mappings

| Go Type    | C++ Type          |
|------------|-------------------|
| `int`      | `int` (via alias)  |
| `int8`     | `std::int8_t`     |
| `int16`    | `std::int16_t`    |
| `int32`    | `std::int32_t`    |
| `int64`    | `std::int64_t`    |
| `uint8`    | `std::uint8_t`    |
| `uint16`   | `std::uint16_t`   |
| `uint32`   | `std::uint32_t`   |
| `uint64`   | `std::uint64_t`   |
| `float32`  | `float`           |
| `float64`  | `double`          |
| `bool`     | `bool`            |
| `string`   | `std::string`     |
| `any`      | `std::any`        |

Type aliases are emitted at the top of the file for convenience:

```cpp
using int8 = int8_t;
using int16 = int16_t;
using int32 = int32_t;
using int64 = int64_t;
using uint8 = uint8_t;
using uint16 = uint16_t;
using uint32 = uint32_t;
using uint64 = uint64_t;
using float32 = float;
using float64 = double;
```

`nil` is emitted as `{}` (value-initialization).

---

## 2. Headers

Every generated file includes:

```cpp
#include <vector>
#include <string>
#include <tuple>
#include <any>
#include <cstdint>
#include <functional>
#include <cstdarg>
#include <initializer_list>
#include <iostream>
```

When `--link-runtime` is enabled, a graphics runtime header is also included (selected via `--graphics-runtime`):

```cpp
#include "graphics/cpp/graphics_runtime_tigr.hpp"   // default: tigr
#include "graphics/cpp/graphics_runtime_sdl2.hpp"    // alternative: sdl2
```

---

## 3. Variable Declarations

### Short declarations (`:=`)

```go
x := 5
```
```cpp
auto x = 5;
```

When the RHS is a string literal, `std::string` is used instead of `auto` to avoid `const char*`:

```go
s := "hello"
```
```cpp
std::string s = "hello";
```

### Multi-value declarations

```go
a, b := compute()
```
```cpp
auto [a, b] = compute();
```

### Regular assignment

```go
a, b = compute()
```
```cpp
std::tie(a, b) = compute();
```

### Constants

```go
const MaxSize = 1024
```
```cpp
constexpr auto MaxSize = 1024;
```

---

## 4. Structs

### Declaration

```go
type CPU struct {
    A      uint8
    PC     int
    Memory []uint8
}
```
```cpp
struct CPU {
    uint8_t A;
    int PC;
    std::vector<uint8_t> Memory;
};
```

All fields are public by default (C++ struct). No derive macros — C++ provides implicit copy/move constructors.

### Initialization

```go
p := Point{x: 10, y: 20}
```
```cpp
auto p = Point{.x = 10, .y = 20};
```

C++20 designated initializers are used for named field initialization.

---

## 5. Slices and Arrays

### Type mapping

```go
var mem []uint8
```
```cpp
std::vector<uint8_t> mem;
```

Both Go slices (`[]T`) and arrays (`[n]T`) map to `std::vector<T>`.

### Composite literals

```go
data := []int{1, 2, 3}
```
```cpp
auto data = std::vector<int>{1, 2, 3};
```

### Indexing

```go
x := data[i]
```
```cpp
auto x = data[i];
```

No `as usize` cast needed — C++ allows integer indices directly.

### Slicing

```go
sub := data[a:b]
```
```cpp
auto sub = std::vector<std::remove_reference<decltype(data[0])>::type>(data.begin() + a, data.begin() + b);
```

`std::remove_reference<decltype(...)>::type` deduces the element type. Iterator arithmetic selects the range.

### Length

```go
n := len(data)
```
```cpp
auto n = std::size(data);
```

---

## 6. Strings

### Literals

```go
s := "hello"
```
```cpp
std::string s = "hello";
```

Raw strings:
```go
s := `raw\nstring`
```
```cpp
auto s = R"(raw\nstring)";
```

### Concatenation

Uses C++ `std::string` `operator+` directly (no special handling needed).

### Indexing

```go
b := s[0]
```
```cpp
auto b = s[0];
```

---

## 7. Functions

### Declaration

```go
func Step(c CPU) CPU {
    // body
}
```
```cpp
CPU Step(CPU c) {
    // body
}
```

Return type comes before the function name (C++ syntax). Parameters are passed by value.

### Multi-value returns

```go
func FetchByte(c CPU) (CPU, uint8) {
    return c, value
}
```
```cpp
std::tuple<CPU, uint8_t> FetchByte(CPU c) {
    return std::make_tuple(c, value);
}
```

Call site:
```go
c, val := FetchByte(c)
```
```cpp
auto [c, val] = FetchByte(c);
```

### Forward declarations

The emitter performs a forward-declaration pass before emitting function bodies. This allows functions to call each other regardless of definition order.

---

## 8. Copy Semantics

C++ uses implicit value semantics — passing a struct by value invokes the copy constructor. This naturally matches Go's copy-on-pass behavior with no explicit annotation.

```go
c = Step(c)
```
```cpp
c = Step(c);   // implicit copy via copy constructor
```

For large structs (e.g., CPU with 64KB `std::vector<uint8_t>`), this implicit copy is expensive. The `--optimize-moves` flag enables `std::move()` where safe:

```cpp
c = Step(std::move(c));   // zero-cost move (with --optimize-moves)
```

The move optimization applies when:
1. The variable appears on both LHS and in call arguments.
2. The variable appears only once across all arguments.
3. The code is not inside a closure (`funcLitDepth == 0`).

Multi-value return optimization:
```go
return c, value
```
```cpp
// Before:  return std::make_tuple(c, value);            // copies c
// After:   return std::make_tuple(std::move(c), value);  // moves c
```

This applies when later results don't reference the first result's identifier.

---

## 9. Control Flow

### If / Else

```go
if x > 0 {
} else {
}
```
```cpp
if (x > 0) {
} else {
}
```

### For Loops

#### C-style

```go
for i := 0; i < 10; i++ {
}
```
```cpp
for (auto i = 0; i < 10; i++) {
}
```

#### Condition-only

```go
for condition {
}
```
```cpp
for (; condition;) {
}
```

#### Infinite

```go
for {
}
```
```cpp
for (;;) {
}
```

#### Range (single value)

```go
for x := range items {
}
```
```cpp
for (auto x : items) {
}
```

#### Range (key-value)

```go
for i, v := range items {
}
```
```cpp
for (size_t i = 0; i < items.size(); i++) {
    auto v = items[i];
    // body
}
```

The value declaration is emitted at the start of the loop body as a pending declaration.

### Increment / Decrement

```go
i++
i--
```
```cpp
i++;
i--;
```

Inside for-loop post-conditions, the trailing semicolon is suppressed.

### Switch / Case

```go
switch x {
case 1:
    doA()
case 2:
    doB()
default:
    doC()
}
```
```cpp
switch (x) {
    case 1:
        doA();
        break;
    case 2:
        doB();
        break;
    default:
        doC();
        break;
}
```

Go's implicit break (no fallthrough) is preserved by emitting `break;` after each case body.

### Break / Continue

Direct 1-to-1 mapping.

---

## 10. Expressions

### Binary operators

All Go operators map directly: `+`, `-`, `*`, `/`, `%`, `==`, `!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `&`, `|`, `^`, `<<`, `>>`

### Unary operators

```go
!b
-x
```
```cpp
(!b)
(-x)
```

### Type conversions

Go type conversions (`int8(x)`) are emitted as C++ function-style casts:

```go
int8(value)
```
```cpp
int8(value)
```

The type names from `cppTypesMap` are substituted.

### Type assertions (`interface{}`)

```go
val := x.(string)
```
```cpp
auto val = std::any_cast<std::string>(std::any(x));
```

### Parenthesized expressions

```go
(a + b)
```
```cpp
(a + b)
```

---

## 11. Closures / Function Literals

```go
handler := func(w Window) bool {
    return true
}
```
```cpp
auto handler = [&](Window w) -> bool {
    return true;
};
```

`[&]` captures all outer variables by reference. The `funcLitDepth` counter tracks closure nesting to disable move optimizations inside closures (captured variables cannot be moved out of `[&]` capture).

---

## 12. Packages and Namespaces

### Non-main packages

```go
package cpu
```
```cpp
namespace cpu {
    // declarations
} // namespace cpu
```

### Package-qualified access

```go
cpu.Step(c)
```
```cpp
cpu::Step(c);
```

If the identifier is a namespace, `::` is used. Otherwise `.` is used for member access.

### Built-in package lowering

`fmt` is lowered — the package qualifier is removed and functions are mapped to top-level helpers.

---

## 13. Built-in Functions

### `len`

```go
len(items)
```
```cpp
std::size(items)
```

### `append`

Emitted as template overloads:

```cpp
// Single element
template <typename T>
std::vector<T>& append(std::vector<T>& vec, const T& element) {
    vec.push_back(element);
    return vec;
}

// Initializer list
template <typename T>
std::vector<T>& append(std::vector<T>& vec, const std::initializer_list<T>& elements) {
    vec.insert(vec.end(), elements);
    return vec;
}

// Vector
template <typename T>
std::vector<T>& append(std::vector<T>& vec, const std::vector<T>& elements) {
    vec.insert(vec.end(), elements.begin(), elements.end());
    return vec;
}

// const char* to string vector
std::vector<std::string>& append(std::vector<std::string>& vec, const char* element) {
    vec.push_back(std::string(element));
    return vec;
}
```

### `fmt.Println`

```go
fmt.Println(x)
```
```cpp
println(x);
```

Emitted helper:
```cpp
template<typename T>
void println(const T& val) { std::cout << val << std::endl; }

void println() { printf("\n"); }
```

### `fmt.Printf`

```go
fmt.Printf("value: %d\n", x)
```
```cpp
printf("value: %d\n", x);
```

Uses C-style `printf` directly (from `<cstdio>` via `<iostream>`).

### `fmt.Sprintf`

```go
s := fmt.Sprintf("value: %d", x)
```
```cpp
auto s = string_format("value: %d", x);
```

Emitted helper using `vsnprintf`:
```cpp
std::string string_format(const std::string fmt, ...) {
    int size = ((int)fmt.size()) * 2 + 50;
    std::string str;
    va_list ap;
    while (1) {
        str.resize(size);
        va_start(ap, fmt);
        int n = vsnprintf((char*)str.data(), size, fmt.c_str(), ap);
        va_end(ap);
        if (n > -1 && n < size) { str.resize(n); return str; }
        if (n > -1) size = n + 1; else size *= 2;
    }
    return str;
}
```

---

## 14. Interfaces

```go
var x interface{} = 42
```
```cpp
std::any x = 42;
```

Type assertion:
```go
val := x.(int)
```
```cpp
auto val = std::any_cast<int>(std::any(x));
```

---

## 15. Assignment Operators

| Go   | C++  |
|------|------|
| `=`  | `=`  |
| `:=` | `auto ... =` |
| `+=` | `+=` |
| `-=` | `-=` |
| `*=` | `*=` |
| `/=` | `/=` |

---

## Summary of Ownership Rules

| Context                          | Go Behavior           | C++ Emission                     |
|----------------------------------|-----------------------|----------------------------------|
| Pass struct to function          | Implicit copy         | Implicit copy (copy constructor) |
| Pass string to function          | Implicit copy         | Implicit copy                    |
| Pass slice to function           | Reference share       | Implicit copy (`std::vector`)    |
| Pass primitive to function       | Copy                  | Copy                             |
| Pass to `append()`               | Ownership transfer    | Reference parameter (`&`)        |
| Return struct in tuple           | Copy                  | `std::make_tuple(c, ...)`        |
| With `--optimize-moves`          | -                     | `std::move()` where safe         |
