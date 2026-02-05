# Go to C# Transpilation Rules

This document describes the baseline rules the ULang transpiler uses to convert Go source code into C#. It covers type mappings, control flow, expressions, declarations, and built-in functions.

---

## 1. Primitive Type Mappings

| Go Type    | C# Type   |
|------------|-----------|
| `int`      | `int`     |
| `int8`     | `sbyte`   |
| `int16`    | `short`   |
| `int32`    | `int`     |
| `int64`    | `long`    |
| `uint8`    | `byte`    |
| `uint16`   | `ushort`  |
| `uint32`   | `uint`    |
| `uint64`   | `ulong`   |
| `float32`  | `float`   |
| `float64`  | `double`  |
| `bool`     | `bool`    |
| `string`   | `string`  |
| `any`      | `object`  |

`nil` is emitted as `default` (C# default value expression).

---

## 2. Project Structure

### Using directives

Every generated file starts with:

```csharp
using System;
using System.Collections;
using System.Collections.Generic;
```

### Package mapping

Each Go package becomes a `public static class`:

```go
package cpu
```
```csharp
public static class cpu {
    // all types and functions
}
```

The `main` package becomes `MainClass`.

### .csproj generation

When `--link-runtime` is enabled, a `.csproj` file is generated:

```xml
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <AllowUnsafeBlocks>true</AllowUnsafeBlocks>
  </PropertyGroup>
</Project>
```

Graphics backend targets (tigr or sdl2) are added when applicable.

---

## 3. Variable Declarations

### Short declarations (`:=`)

```go
x := 5
```
```csharp
var x = 5;
```

### Multi-value declarations

```go
a, b := compute()
```
```csharp
var (a, b) = compute();
```

### Explicit `var` declarations

```go
var x int8
```
```csharp
sbyte x = default;
```

The `default` keyword provides C#'s zero-value initialization.

### Slice variable initialization

```go
var items []int
```
```csharp
List<int> items = new List<int>();
```

### Regular assignment

```go
a, b = compute()
```
```csharp
(a, b) = compute();
```

### Compound assignments

`+=`, `-=`, `*=`, `/=` are preserved as-is.

### Blank identifier

When all LHS identifiers are `_`, the entire statement is suppressed.

---

## 4. Structs

### Declaration

```go
type Person struct {
    Name string
    Age  int
}
```
```csharp
public struct Person {
    public string Name;
    public int Age;
};
```

C# structs are value types — assignment creates a copy, matching Go's semantics.

### Initialization

```go
p := Person{Name: "Alice", Age: 30}
```
```csharp
var p = new Person{Name = "Alice", Age = 30};
```

---

## 5. Slices and Arrays

### Type mapping

```go
var items []int
var nested [][]int
```
```csharp
List<int> items = new List<int>();
List<List<int>> nested = new List<List<int>>();
```

Go slices (`[]T`) map to C# `List<T>`. Nested slices are recursively converted.

### Indexing

```go
x := items[i]
```
```csharp
var x = items[i];
```

### Slicing

```go
sub := items[a:b]
```
```csharp
var sub = items[a..b];
```

Uses C# 8.0 range syntax.

### Length

```go
n := len(items)
```
```csharp
var n = SliceBuiltins.Length(items);
```

### Append

```go
items = append(items, 42)
```
```csharp
items = SliceBuiltins.Append(items, 42);
```

---

## 6. Strings

### Literals

```go
s := "hello"
```
```csharp
var s = "hello";
```

Raw strings:
```go
s := `raw\nstring`
```
```csharp
var s = @"raw\nstring";
```

C# verbatim string (`@"..."`) preserves backslashes as literal characters.

### Length

```go
n := len(s)
```
```csharp
var n = SliceBuiltins.Length(s);
```

The `SliceBuiltins.Length` has a string overload returning `s.Length`.

---

## 7. Functions

### Declaration

```go
func Add(a int, b int) int {
    return a + b
}
```
```csharp
public static int Add(int a, int b) {
    return (a + b);
}
```

All functions are `public static`. Return type precedes the function name.

The `main` function is renamed to `Main`:
```csharp
public static void Main() { ... }
```

### Multi-value returns

```go
func DivMod(a, b int) (int, int) {
    return a / b, a % b
}
```
```csharp
public static (int, int) DivMod(int a, int b) {
    return ((a / b), (a % b));
}
```

C# value tuples `(T1, T2)` map Go multi-value returns.

Call site:
```go
q, r := DivMod(10, 3)
```
```csharp
var (q, r) = DivMod(10, 3);
```

### Forward declarations

A forward-declaration pass runs first (`forwardDecls = true`), skipping bodies. The actual emission pass follows.

---

## 8. Copy Semantics

C# structs are value types — passing to a function copies. This naturally matches Go's value semantics:

```go
c = Step(c)
```
```csharp
c = Step(c);   // struct copy
```

C# `List<T>` is a reference type, so slice semantics differ from Go. The `SliceBuiltins.Append` method returns a new list to preserve functional-style ownership.

No move optimization is implemented for the C# backend.

---

## 9. Control Flow

### If / Else

```go
if x > 0 {
} else {
}
```
```csharp
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
```csharp
for (var i = 0; (i < 10); i++) {
}
```

#### Infinite

```go
for {
}
```
```csharp
for (;;) {
}
```

#### Range (single value)

```go
for item := range items {
}
```
```csharp
foreach (var item in items) {
}
```

#### Range (key-value)

```go
for i, v := range items {
}
```
```csharp
for (int i = 0; i < items.Count; i++) {
    var v = items[i];
    // body
}
```

The key-value range is converted to an index-based `for` loop. The value variable declaration is emitted at the start of the loop body.

### Increment / Decrement

```go
i++
i--
```
```csharp
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
```csharp
switch ((x)) {
    case (int)(1):
        doA();
        break;
    case (int)(2):
        doB();
        break;
    default:
        doC();
        break;
}
```

Go's implicit break (no fallthrough) is preserved by emitting `break;` after each case. Case values are cast to `(int)` to match the tag type.

### Break / Continue

Direct 1-to-1 mapping.

---

## 10. Expressions

### Binary operators

All Go operators map directly: `+`, `-`, `*`, `/`, `%`, `==`, `!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `&`, `|`, `^`, `<<`, `>>`

Expressions are wrapped in parentheses:
```go
a + b
```
```csharp
((a + b))
```

### Unary operators

```go
!b
-x
```
```csharp
((!b))
((-x))
```

### Type conversions

```go
int8(value)
```
```csharp
(sbyte)(value)
```

Go type conversions are emitted as C# cast expressions.

---

## 11. Closures / Function Literals

```go
handler := func(x int) int {
    return x * 2
}
```
```csharp
var handler = (int x) => {
    return (x * 2);
};
```

C# lambda syntax with `=>`. Parameters include type annotations.

### Function types

Functions as parameters use `Action<>` (no return) or `Func<>` (with return):

```go
func apply(f func(int) int, x int) int
```
```csharp
public static int apply(Func<int, int> f, int x)
```

---

## 12. Interfaces

```go
var x interface{} = 42
```
```csharp
object x = 42;
```

Go's `interface{}` / `any` maps to C# `object`, which is the base type of all types. Boxing handles value types automatically.

Type assertions are not currently supported in the C# backend.

---

## 13. Built-in Functions

### `SliceBuiltins` class

Emitted at the top of the file:

```csharp
public static class SliceBuiltins {
    public static int Length<T>(ICollection<T> collection) {
        return collection == null ? 0 : collection.Count;
    }
    public static int Length(string s) {
        return s == null ? 0 : s.Length;
    }
    public static List<T> Append<T>(this List<T> list, T element) { ... }
    public static List<T> Append<T>(this List<T> list, params T[] elements) { ... }
    public static List<T> Append<T>(this List<T> list, List<T> elements) { ... }
}
```

### `fmt.Println`

```go
fmt.Println(x)
```
```csharp
Console.WriteLine(x);
```

### `fmt.Printf` / `fmt.Print`

```go
fmt.Printf("value: %d\n", x)
```
```csharp
Formatter.Printf("value: %d\n", x);
```

The `Formatter` class converts Go format specifiers (`%d`, `%s`, `%f`, `%c`) to C# format strings using `string.Format`.

### `fmt.Sprintf`

```go
s := fmt.Sprintf("value: %d", x)
```
```csharp
var s = Formatter.Sprintf("value: %d", x);
```

---

## 14. Type Aliases

Go type aliases are tracked and converted to their underlying C# types:

```go
type AST []Statement
```

The alias is resolved at all usage sites. `[]Statement` becomes `List<Statement>` everywhere `AST` is used.

---

## 15. Constants

```go
const MaxSize = 1024
```

Constants are emitted inside the static class. In namespace context they become properties with `,` separators.

---

## 16. Assignment Operators

| Go   | C#   |
|------|------|
| `=`  | `=`  |
| `:=` | `var ... =` |
| `+=` | `+=` |
| `-=` | `-=` |
| `*=` | `*=` |
| `/=` | `/=` |

---

## Summary of Ownership Rules

| Context                          | Go Behavior           | C# Emission                      |
|----------------------------------|-----------------------|----------------------------------|
| Pass struct to function          | Implicit copy         | Value-type copy (struct)         |
| Pass string to function          | Implicit copy         | Reference (string is immutable)  |
| Pass slice to function           | Reference share       | Reference (`List<T>`)            |
| Pass primitive to function       | Copy                  | Value-type copy                  |
| Pass to `append()`               | Returns new slice     | `SliceBuiltins.Append` returns new list |
| Return struct in tuple           | Copy                  | Value tuple `(T1, T2)`          |
| Assign struct                    | Copy                  | Value-type copy                  |
