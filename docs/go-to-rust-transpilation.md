# Go to Rust Transpilation Rules

This document describes the baseline rules the ULang transpiler uses to convert Go source code into Rust. It covers type mappings, control flow, expressions, declarations, clone generation, and built-in functions. Optimization passes are described separately in `docs/move-optimization.md`.

---

## 1. Primitive Type Mappings

| Go Type    | Rust Type      |
|------------|----------------|
| `int`      | `i32`          |
| `int8`     | `i8`           |
| `int16`    | `i16`          |
| `int32`    | `i32`          |
| `int64`    | `i64`          |
| `uint8`    | `u8`           |
| `uint16`   | `u16`          |
| `uint32`   | `u32`          |
| `uint64`   | `u64`          |
| `float32`  | `f32`          |
| `float64`  | `f64`          |
| `bool`     | `bool`         |
| `string`   | `String`       |
| `any`      | `Box<dyn Any>` |

Go `int` always maps to `i32` (not platform-dependent). User-defined type names that are not in this table pass through unchanged.

---

## 2. Keyword Escaping

Rust reserved keywords appearing as identifiers are prefixed with `r#`:

```
as, break, const, continue, crate, else, enum, extern, fn, for, if,
impl, in, let, loop, match, mod, move, mut, pub, ref, return, self,
Self, static, struct, super, trait, type, unsafe, use, where, while,
abstract, async, await, become, box, do, final, macro, override, priv,
try, typeof, unsized, virtual, yield
```

Example: a Go variable named `type` becomes `r#type` in Rust.

Boolean literals `true` and `false` are NOT escaped.

---

## 3. Variable Declarations

### `var` declarations

```go
var x int
```
```rust
let mut x: i32 = i32::default();
```

All `var` declarations use `let mut` because Go variables are mutable by default. When no initial value is given, the Rust `Default` trait provides a zero value.

Multiple names in one declaration expand into separate `let` statements:

```go
var x, y int
```
```rust
let mut x: i32 = i32::default();
let mut y: i32 = i32::default();
```

### Short declarations (`:=`)

Single value:
```go
x := 5
```
```rust
let mut x = 5;
```

Multi-value (tuple destructuring):
```go
x, y := compute(5)
```
```rust
let (mut x, mut y) = compute(5);
```

### Constants

```go
const MaxSize int = 1024
```
```rust
pub const MaxSize: i32 = 1024;
```

Constants are the only immutable bindings.

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
```rust
#[derive(Default, Clone, Debug)]
pub struct CPU {
    pub A: u8,
    pub PC: i32,
    pub Memory: Vec<u8>,
}
```

All fields become `pub`. Field order is preserved but syntax changes from `name type` to `name: type`.

### Derive Macros

The derive macros depend on the field types:

| Field types                        | Derive                              |
|------------------------------------|-------------------------------------|
| All Copy types (primitives, bool)  | `Default, Clone, Copy, Debug`       |
| Contains `Vec`, `String`, structs  | `Default, Clone, Debug`             |
| Contains `interface{}` / `any`     | `Debug` only                        |
| Contains function fields           | `Clone` only                        |

Structs that derive `Copy` allow the compiler to use bitwise copies instead of `Clone`.

### Initialization

Partial initialization uses `Default::default()` for missing fields:

```go
c := CPU{A: 0, PC: 0x0600}
```
```rust
let mut c = CPU{A: 0, PC: 0x0600, ..Default::default()};
```

Empty struct initialization:
```go
c := CPU{}
```
```rust
let mut c = CPU{..Default::default()};
```

Structs with `interface{}` or function fields cannot use `Default::default()` (not derivable) and must specify all fields.

---

## 5. Slices and Arrays

### Type mapping

Both Go slices (`[]T`) and arrays (`[n]T`) map to Rust `Vec<T>`:

```go
var mem []uint8
var arr [256]int
```
```rust
let mut mem: Vec<u8> = Vec::new();
let mut arr: Vec<i32> = Vec::new();
```

### Composite literals

Empty slice:
```go
data := []int{}
```
```rust
let mut data: Vec<i32> = Vec::new();
```

Non-empty slice:
```go
data := []int{1, 2, 3}
```
```rust
let mut data = vec![1, 2, 3];
```

### Indexing

```go
x := mem[i]
```
```rust
let mut x = mem[i as usize];
```

Go integer indices are cast to `usize` because Rust requires `usize` for indexing. The cast is emitted for `int`, `int8`, `int16`, `int32`, and `int64`.

For string indexing, `.as_bytes()` is inserted:
```go
b := str[i]
```
```rust
let mut b = str.as_bytes()[i as usize];
```

### Slicing

```go
sub := data[a:b]
```
```rust
let mut sub = data[a as usize..b as usize].to_vec();
```

The slice produces a reference (`&[T]`), and `.to_vec()` converts it back to an owned `Vec<T>`.

---

## 6. Strings

### Literals

```go
s := "hello"
```
```rust
let mut s = "hello".to_string();
```

Rust string literals are `&str`. The `.to_string()` call converts to owned `String`.

Raw strings:
```go
s := `raw string`
```
```rust
let mut s = r#"raw string"#.to_string();
```

The `.to_string()` suffix is NOT added in `+=` context (Rust `+=` expects `&str`).

### Concatenation

```go
result := s1 + s2
```
```rust
let mut result = (s1 + &s2);
```

Rust's `String + &str` operator requires the right operand to be a reference.

### Indexing

```go
b := str[0]
```
```rust
let mut b = str.as_bytes()[0 as usize];
```

### Length

See [Built-in Functions](#11-built-in-functions).

---

## 7. Functions

### Declaration

```go
func Step(c CPU) CPU {
    // body
}
```
```rust
pub fn Step(mut c: CPU) -> CPU {
    // body
}
```

All parameters are `mut` (Go variables can always be reassigned). All functions are `pub`.

### Multi-value returns

```go
func FetchByte(c CPU) (CPU, uint8) {
    return c, value
}
```
```rust
pub fn FetchByte(mut c: CPU) -> (CPU, u8) {
    return (c, value);
}
```

Multiple return values become a Rust tuple. The call site destructures:

```go
c, val := FetchByte(c)
```
```rust
let (mut c, mut val) = FetchByte(c);
```

### Package-qualified calls

```go
cpu.Step(c)
```
```rust
cpu::Step(c)
```

Go's `.` for package access becomes Rust's `::` for module access. Field access retains `.`.

---

## 8. Clone Generation (Baseline)

Go uses value semantics — passing a struct to a function creates an independent copy. Rust moves values by default, invalidating the original. The emitter adds `.clone()` to preserve Go's copy semantics.

### Where `.clone()` is generated

#### 8.1 Function call arguments

Every non-Copy type argument to a function call gets `.clone()`:

**Structs:**
```go
c = Step(c)
```
```rust
c = Step(c.clone());
```

**Strings:**
```go
process(name)
```
```rust
process(name.clone());
```

String *literals* are exempt (they already produce owned `String` via `.to_string()`).

**Slices/Vecs:**
```go
result := transform(data)
```
```rust
let mut result = transform(data.clone());
```

Slice arguments to `append()` are exempt (append takes ownership).

**Structs with `interface{}` fields** are NOT cloned (`Box<dyn Any>` does not implement `Clone`).

#### 8.2 Multi-value return statements

The first result in a multi-value return is cloned when it is an identifier:

```go
return c, value
```
```rust
return (c.clone(), value);
```

This prevents "use of moved value" errors when later results might reference the same variable (e.g., `return c, c.Memory[addr]`).

#### 8.3 Vec element access

Accessing a non-Copy element from a Vec produces `.clone()`:

```go
line := lines[i]
```
```rust
let mut line = lines[i as usize].clone();
```

Rust doesn't allow moving out of indexed collections. This applies to struct and string elements. Not applied when on the left-hand side of an assignment.

#### 8.4 Simple variable assignment

Assigning a non-Copy variable to another variable clones:

```go
backup = current
```
```rust
backup = current.clone();
```

Applies to strings, slices, and structs. Constants are exempt.

#### 8.5 Range loop collections

```go
for v := range items {
```
```rust
for v in items.clone() {
```

The collection is cloned so the original remains valid after the loop consumes the iterator.

#### 8.6 Key-value range loops

```go
for i, v := range items {
```
```rust
for (i, v) in items.clone().iter().enumerate() {
```

#### 8.7 Struct field initialization

String and Vec fields in struct composite literals clone the source variable:

```go
s := MyStruct{name: existingName}
```
```rust
let mut s = MyStruct{name: existingName.clone(), ..Default::default()};
```

### Types exempt from `.clone()`

Copy types never need cloning — the Rust compiler copies them implicitly:

- `bool`
- `i8`, `i16`, `i32`, `i64`
- `u8`, `u16`, `u32`, `u64`
- `f32`, `f64`
- Structs that derive `Copy` (all fields are also Copy)

---

## 9. Control Flow

### If / Else

Direct 1-to-1 mapping:

```go
if x > 0 {
    // ...
} else if x < 0 {
    // ...
} else {
    // ...
}
```
```rust
if (x > 0) {
    // ...
} else if (x < 0) {
    // ...
} else {
    // ...
}
```

### For Loops

Go has a single `for` keyword covering several patterns. The emitter analyzes the loop structure and produces the appropriate Rust form.

#### Infinite loop

```go
for {
    // ...
}
```
```rust
loop {
    // ...
}
```

#### Condition-only (while)

```go
for condition {
    // ...
}
```
```rust
while condition {
    // ...
}
```

#### C-style with simple bounds → Rust range

```go
for i := 0; i < n; i++ {
    // ...
}
```
```rust
for i in 0..n {
    // ...
}
```

With `<=`:
```go
for i := 0; i <= n; i++ {
    // ...
}
```
```rust
for i in 0..=n {
    // ...
}
```

#### Reverse loop

```go
for i := n; i > 0; i-- {
    // ...
}
```
```rust
for i in (1..=n).rev() {
    // ...
}
```

#### Step > 1

```go
for i := 0; i < n; i += 2 {
    // ...
}
```
```rust
for i in (0..n).step_by(2) {
    // ...
}
```

#### Compound condition → while with manual increment

When the condition contains `&&` or `||`, a range expression cannot represent it. The emitter falls back to a `while` loop with explicit initialization and increment:

```go
for i := 0; i < n && active; i++ {
    // ...
}
```
```rust
let mut i = 0;
while (i < n) && active {
    // ...
    i += 1;
}
```

The increment statement is appended at the end of the loop body.

#### Range (single value)

```go
for v := range items {
    // ...
}
```
```rust
for v in items.clone() {
    // ...
}
```

For strings:
```go
for c := range str {
    // ...
}
```
```rust
for c in str.bytes() {
    // ...
}
```

#### Range (key-value)

```go
for i, v := range items {
    // ...
}
```
```rust
for (i, v) in items.clone().iter().enumerate() {
    // ...
}
```

For strings:
```go
for i, c := range str {
    // ...
}
```
```rust
for (i, c) in str.bytes().enumerate() {
    // ...
}
```

### Increment / Decrement

```go
i++
i--
```
```rust
i += 1;
i -= 1;
```

### Switch / Match

```go
switch x {
case 1:
    // ...
case 2:
    // ...
default:
    // ...
}
```
```rust
match x {
    1 => {
        // ...
    }
    2 => {
        // ...
    }
    _ => {
        // ...
    }
}
```

If the switch tag is a small integer type (`int8`, `uint8`, `int16`, `uint16`), it is cast to `i32` to match Rust constant types:

```rust
match x as i32 {
```

If no `default` case exists, an empty `_ => {}` arm is added for Rust match exhaustiveness.

### Break / Continue

Direct 1-to-1 mapping.

---

## 10. Expressions

### Binary operators

All Go arithmetic and logical operators map directly to Rust:

`+`, `-`, `*`, `/`, `%`, `==`, `!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `&`, `|`, `^`, `<<`, `>>`

All binary expressions are wrapped in parentheses:
```go
a + b * c
```
```rust
(a + (b * c))
```

### Type comparison casting

When comparing a small integer type to an `i32` constant, the left operand is cast:

```go
var x uint8 = 5
if x == 10 { ... }
```
```rust
if ((x as i32) == 10) { ... }
```

### String concatenation

The right operand is borrowed:
```go
s1 + s2
```
```rust
(s1 + &s2)
```

### Unary operators

```go
!x
-x
```
```rust
(!x)
(-x)
```

### Type conversions

Go type conversions (which are `CallExpr` nodes in the AST) become Rust `as` casts:

```go
int(addr)
uint8(value)
float64(x)
```
```rust
(addr as i32)
(value as u8)
(x as f64)
```

### Integer literals in float context

An integer literal used where a float is expected gets a decimal point:
```go
var f float64 = 5
```
```rust
let mut f: f64 = 5.0;
```

---

## 11. Built-in Functions

The emitter emits Rust helper functions at the top of each output file.

### `len`

```go
n := len(slice)
```
```rust
let mut n = len(&slice);
```

The `&` is prepended automatically. Emitted helper:
```rust
pub fn len<T>(slice: &[T]) -> i32 {
    slice.len() as i32
}
```

Returns `i32` to match Go's `int` return type.

### `append`

```go
slice = append(slice, value)
```
```rust
slice = append(slice, value);
```

Emitted helper (takes ownership, no clone needed):
```rust
pub fn append<T>(mut vec: Vec<T>, value: T) -> Vec<T> {
    vec.push(value);
    vec
}
```

Multi-value append variant:
```rust
pub fn append_many<T: Clone>(mut vec: Vec<T>, values: &[T]) -> Vec<T> {
    vec.extend_from_slice(values);
    vec
}
```

### `fmt.Println`

```go
fmt.Println("hello", x)
```
```rust
println!("{} {}", "hello", x);
```

### `fmt.Printf`

```go
fmt.Printf("value: %d\n", x)
```
```rust
printf("value: %d\n".to_string(), x);
```

Emitted helper replaces `%d`, `%s`, `%v` with `{}` and uses Rust's `format!`:
```rust
pub fn printf<T: fmt::Display>(fmt_str: String, v1: T) { ... }
```

Variants for 2, 3, 4 arguments: `printf3`, `printf4`, `printf5`.

### `fmt.Sprintf`

```go
s := fmt.Sprintf("value: %d", x)
```
```rust
let mut s = string_format2("value: %d", x);
```

Emitted helper:
```rust
pub fn string_format2<T: fmt::Display>(fmt_str: &str, val: T) -> String {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    rust_fmt.replace("{}", &format!("{}", val))
}
```

---

## 12. Interfaces and Type Assertions

### `interface{}` / `any`

All empty interfaces map to `Box<dyn Any>`:

```go
var x interface{} = 42
```
```rust
let mut x: Box<dyn Any> = Box::new(42);
```

When returning a concrete value from a function that returns `interface{}`, the value is wrapped in `Box::new()`.

### Type assertions

```go
val := x.(int)
```
```rust
let mut val = x.downcast_ref::<i32>().unwrap().clone();
```

The result is cloned because `downcast_ref` returns a reference.

---

## 13. Packages and Modules

### Package declaration

The `main` package has no module wrapper. Non-main packages become Rust modules:

```go
package cpu
```
```rust
pub mod cpu {
    use crate::*;
    // ... declarations
}
```

`use crate::*` makes all crate-level items available inside the module.

### Package-qualified access

```go
cpu.Step(c)
```
```rust
cpu::Step(c)
```

Go's `.` for package member access becomes `::`. Field access on values retains `.`.

### Built-in package lowering

Some Go standard library functions are lowered to crate-level helpers:

| Go                 | Rust                  |
|--------------------|-----------------------|
| `fmt.Println()`    | `println!()`          |
| `fmt.Printf()`     | `printf()`            |
| `fmt.Sprintf()`    | `string_format2()`    |

The `fmt` package qualifier is removed; the functions are emitted as top-level helpers.

---

## 14. Closures / Function Literals

```go
handler := func(w Window) bool {
    // body
    return true
}
```
```rust
let handler = |mut w: Window| -> bool {
    // body
    return true;
};
```

Go function literals become Rust closures using `|args| -> ReturnType { body }` syntax.

### Local closure inlining

When a closure is assigned to a local variable and then called, the emitter can inline the closure body at the call site rather than storing it as a variable. This avoids Rust closure capture issues.

### Closure depth tracking

The emitter tracks `funcLitDepth` — the nesting depth inside function literals. This is used to determine whether captured variables can be moved (they cannot inside `FnMut` closures in Rust, requiring `.clone()` instead).

---

## 15. Assignment Operators

| Go   | Rust |
|------|------|
| `=`  | `=`  |
| `:=` | `let mut ... =` |
| `+=` | `+=` |
| `-=` | `-=` |
| `*=` | `*=` |
| `/=` | `/=` |

Multi-value assignment:
```go
a, b = compute()
```
```rust
(a, b) = compute();
```

Blank identifier:
```go
_, b := compute()
```

When ALL LHS identifiers are `_`, the entire statement is suppressed.

---

## 16. Liveness-Based Clone

Beyond the type-based clone rules in Section 8, the emitter performs a simple liveness analysis. If a non-Copy variable is passed to a function and is also used in later statements within the same function body, `.clone()` is added even if the variable's type wouldn't normally trigger it.

This prevents Rust "use of moved value" errors when a variable is consumed by a function call but referenced again afterward.

---

## Summary of Ownership Rules

| Context                          | Go Behavior           | Rust Emission                    |
|----------------------------------|----------------------|-----------------------------------|
| Pass struct to function          | Implicit copy         | `.clone()`                       |
| Pass string to function          | Implicit copy         | `.clone()`                       |
| Pass slice to function           | Reference share       | `.clone()`                       |
| Pass primitive to function       | Copy                  | (no action — Copy type)          |
| Pass to `append()`               | Ownership transfer    | (no `.clone()` — takes ownership)|
| Pass to `len()`                  | Read-only             | `&` prefix                       |
| Return struct in tuple           | Copy                  | `.clone()` on first result       |
| Access Vec element (non-Copy)    | Reference             | `.clone()` after index           |
| Assign variable                  | Copy                  | `.clone()` for non-Copy types    |
| Range loop collection            | Iteration             | `.clone()` on collection         |
| String literal                   | Value                 | `.to_string()`                   |
