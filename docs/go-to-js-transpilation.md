# Go to JavaScript Transpilation Rules

This document describes the baseline rules the ULang transpiler uses to convert Go source code into JavaScript. It covers type handling, control flow, expressions, declarations, value semantics, and built-in functions.

---

## 1. Type Mappings

JavaScript is dynamically typed — type annotations are suppressed in the output. The type map exists for internal use (e.g., default value generation):

| Go Type    | JS Conceptual Type |
|------------|-------------------|
| `int`      | `number`          |
| `int8`     | `number`          |
| `int16`    | `number`          |
| `int32`    | `number`          |
| `int64`    | `number`          |
| `uint8`    | `number`          |
| `uint16`   | `number`          |
| `uint32`   | `number`          |
| `uint64`   | `number`          |
| `float32`  | `number`          |
| `float64`  | `number`          |
| `bool`     | `boolean`         |
| `string`   | `string`          |
| `any`      | `any`             |

`nil` is emitted as `null`.

### Type conversion functions

Go type conversions are emitted as runtime helper functions:

```javascript
function int8(v)  { return typeof v === 'string' ? v.charCodeAt(0) : (v | 0); }
function int16(v) { return typeof v === 'string' ? v.charCodeAt(0) : (v | 0); }
// ... similar for all integer types
function float32(v) { return v; }
function float64(v) { return v; }
function string(v)  { return String(v); }
function bool(v)    { return Boolean(v); }
```

Integer casts use `| 0` for truncation to integer. String-to-integer uses `charCodeAt(0)`.

---

## 2. Variable Declarations

### Short declarations (`:=`)

```go
x := 5
```
```javascript
let x = 5;
```

### Multi-value declarations

```go
a, b := compute()
```
```javascript
let [a, b] = compute();
```

JavaScript destructuring assignment is used.

### Explicit `var` declarations

```go
var x int
var s string
var items []int
var p Person
```
```javascript
let x = 0;
let s = "";
let items = [];
let p = {Name: "", Age: 0};  // struct fields initialized to zero values
```

Default values are generated based on type:
- Numbers: `0`
- Strings: `""`
- Booleans: `false`
- Slices: `[]`
- Structs: object literal with all fields set to their zero values

### Constants

```go
const MaxSize = 1024
```
```javascript
const MaxSize = 1024;
```

In non-main packages (inside a namespace object), constants become object properties.

---

## 3. Structs

### Declaration

```go
type Person struct {
    Name string
    Age  int
}
```
```javascript
class Person {
    constructor(Name, Age) {
        this.Name = Name;
        this.Age = Age;
    }
}
```

Structs become ES6 classes with a constructor that assigns all fields.

### Initialization

```go
p := Person{Name: "Alice", Age: 30}
```
```javascript
let p = {Name: "Alice", Age: 30};
```

Composite literals produce object literals. Missing fields are filled with zero values.

### In non-main packages

Inside a namespace object, structs are not separate classes — they are created as plain objects with property initializers.

---

## 4. Slices and Arrays

### Type mapping

Go slices map to JavaScript arrays:

```go
var items []int
```
```javascript
let items = [];
```

### Composite literals

```go
data := []int{1, 2, 3}
```
```javascript
let data = [1, 2, 3];
```

### Indexing

```go
x := data[i]
```
```javascript
let x = data[i];
```

### Slicing

```go
sub := data[a:b]
```
```javascript
let sub = data.slice(a, b);
```

Variants:
- `data[a:]` → `data.slice(a)`
- `data[:b]` → `data.slice(0, b)`
- `data[:]` → `data.slice(0)`

### Length

```go
n := len(data)
```
```javascript
let n = len(data);
```

### Append

```go
data = append(data, 42)
```
```javascript
data = append(data, 42);
```

---

## 5. Strings

### Literals

```go
s := "hello"
```
```javascript
let s = "hello";
```

Raw strings use template literals:
```go
s := `raw string`
```
```javascript
let s = `raw string`;
```

### Indexing

Go string indexing returns a byte/rune (numeric). JS uses `charCodeAt`:

```go
b := s[0]
```
```javascript
let b = s.charCodeAt(0);
```

### Concatenation

Uses `+` operator directly (standard in both Go and JS).

---

## 6. Functions

### Declaration

```go
func Add(a int, b int) int {
    return a + b
}
```
```javascript
function Add(a, b) {
    return (a + b);
}
```

No type annotations on parameters or return type.

### Multi-value returns

```go
func DivMod(a, b int) (int, int) {
    return a / b, a % b
}
```
```javascript
function DivMod(a, b) {
    return [(a / b | 0), (a % b)];
}
```

Multi-value returns are wrapped in arrays. Call site uses destructuring:

```go
q, r := DivMod(10, 3)
```
```javascript
let [q, r] = DivMod(10, 3);
```

Note: integer division uses `| 0` for truncation (see Expressions).

---

## 7. Value Semantics

JavaScript objects are reference types. To preserve Go's value semantics for structs, the `append` helper shallow-copies objects:

```javascript
function append(arr, ...items) {
    if (arr == null) arr = [];
    for (const item of items) {
        if (item && typeof item === 'object' && !Array.isArray(item)) {
            arr.push({...item});  // shallow copy to preserve Go value semantics
        } else {
            arr.push(item);
        }
    }
    return arr;
}
```

Object spread (`{...obj}`) is used to create independent copies when appending struct values to arrays.

For other contexts (function calls, assignments), JavaScript's reference semantics apply. This is a known divergence from Go's strict value semantics.

---

## 8. Control Flow

### If / Else

```go
if x > 0 {
} else {
}
```
```javascript
if ((x > 0)) {
} else {
}
```

### For Loops

#### C-style

```go
for i := 0; i < 10; i++ {
}
```
```javascript
for (let i = 0; (i < 10); i++) {
}
```

#### Infinite

```go
for {
}
```
```javascript
while (true) {
}
```

#### Condition-only

```go
for condition {
}
```
```javascript
for (; condition;) {
}
```

#### Range (single value)

```go
for i := range items {
}
```
```javascript
for (let i = 0; i < items.length; i++) {
}
```

#### Range (key-value)

```go
for i, v := range items {
}
```
```javascript
for (let i = 0; i < items.length; i++) {
    let v = items[i];
    // body
}
```

Blank identifier `_` in the key position generates a synthetic `_idx` variable.

### Increment / Decrement

```go
i++
i--
```
```javascript
i++;
i--;
```

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
```javascript
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

Go's implicit break (no fallthrough) is preserved by emitting `break;` after each case.

### Break / Continue

Direct 1-to-1 mapping.

---

## 9. Expressions

### Binary operators

All Go operators map directly: `+`, `-`, `*`, `/`, `%`, `==`, `!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `&`, `|`, `^`, `<<`, `>>`

### Integer division

Go integer division truncates toward zero. JavaScript's `/` produces a float. The emitter adds `| 0` for integer operands:

```go
result := a / b   // both int
```
```javascript
let result = (a / b | 0);
```

### Unary operators

```go
!b
-x
```
```javascript
(!b)
(-x)
```

### Type conversions

Go type conversions call the runtime helper functions:

```go
int8(value)
uint16(x)
```
```javascript
int8(value)
uint16(x)
```

These are the helper functions emitted at the top (see Section 1).

### Type assertions

No-op in JavaScript — the language is dynamically typed, so type assertions are omitted.

---

## 10. Closures / Function Literals

```go
handler := func(x int) int {
    return x * 2
}
```
```javascript
let handler = function(x) {
    return (x * 2);
};
```

Regular `function` syntax (not arrow functions). Inside closures, package-qualified access uses the package name instead of `this` to avoid `this` binding issues.

---

## 11. Packages and Namespaces

### Main package

Functions are emitted at the global scope. A `main()` call is appended at the end of the file:

```javascript
// ... function declarations ...
main();
```

### Non-main packages

Packages become namespace objects:

```go
package containers

func NewList() List {
    return List{}
}
```
```javascript
const containers = {
    NewList: function() {
        return {nodes: [], head: -1};
    },
};
```

### Package-qualified access

```go
containers.NewList()
```
```javascript
containers.NewList()
```

Dot notation works naturally for namespace objects.

---

## 12. Interfaces

```go
var x interface{} = 42
```
```javascript
let x = 42;
```

Go's `interface{}` has no JavaScript equivalent — values are simply assigned without type constraints. Type assertions are omitted (JS is dynamically typed).

---

## 13. Built-in Functions

### Runtime helpers

Emitted at the top of every generated file.

### `len`

```javascript
function len(arr) {
    if (typeof arr === 'string') return arr.length;
    if (Array.isArray(arr)) return arr.length;
    return 0;
}
```

### `append`

```javascript
function append(arr, ...items) {
    if (arr == null) arr = [];
    for (const item of items) {
        if (item && typeof item === 'object' && !Array.isArray(item)) {
            arr.push({...item});
        } else {
            arr.push(item);
        }
    }
    return arr;
}
```

### `make`

```javascript
function make(type, length, capacity) {
    if (Array.isArray(type)) {
        return new Array(length || 0).fill(type[0] === 'number' ? 0 : null);
    }
    return [];
}
```

### `fmt.Println`

```go
fmt.Println(x)
```
```javascript
console.log(x);
```

### `fmt.Printf` / `fmt.Print`

```go
fmt.Printf("value: %d\n", x)
```
```javascript
printf("value: %d\n", x);
```

The `printf` helper handles both Node.js (`process.stdout.write`) and browser (`window._printBuffer`) environments.

### `fmt.Sprintf`

```go
s := fmt.Sprintf("value: %d", x)
```
```javascript
let s = stringFormat("value: %d", x);
```

The `stringFormat` helper supports `%s`, `%d`, `%f`, `%v`, `%x`, `%c` format specifiers using regex replacement.

---

## 14. HTML Generation

When `--link-runtime` is enabled, an HTML file is generated alongside the JS:

```html
<!DOCTYPE html>
<html>
<head>
    <title>appName</title>
    <style>
        body { margin: 0; display: flex; justify-content: center;
               align-items: center; min-height: 100vh; background: #1a1a1a; }
        canvas { border: 1px solid #333; }
    </style>
</head>
<body>
    <script src="appName.js"></script>
</body>
</html>
```

This provides a browser environment for graphics applications using the canvas API.

---

## 15. Assignment Operators

| Go   | JS    |
|------|-------|
| `=`  | `=`   |
| `:=` | `let ... =` |
| `+=` | `+=`  |
| `-=` | `-=`  |
| `*=` | `*=`  |
| `/=` | `/=`  |

---

## Summary of Ownership Rules

| Context                          | Go Behavior           | JS Emission                      |
|----------------------------------|-----------------------|----------------------------------|
| Pass struct to function          | Implicit copy         | Reference (no automatic copy)    |
| Pass string to function          | Implicit copy         | Immutable string (safe)          |
| Pass slice to function           | Reference share       | Reference (array)                |
| Pass primitive to function       | Copy                  | Copy (numbers, booleans)         |
| Pass to `append()`               | Returns new slice     | Mutates array, returns it        |
| Append struct to array           | Copy into array       | Shallow copy via `{...obj}`      |
| Return multiple values           | Tuple of copies       | Array `[a, b]`                   |
| Integer division                 | Truncates to int      | `| 0` for truncation             |
| String indexing                  | Returns byte (number) | `.charCodeAt(i)` (number)        |
