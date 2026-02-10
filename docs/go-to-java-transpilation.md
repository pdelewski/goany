# Java Emitter Documentation

This document describes the Java backend emitter (`compiler/java_emitter.go`) and the challenges involved in transpiling Go code to Java.

## Overview

The Java emitter translates Go source code into equivalent Java code. While both languages are statically typed and compile to bytecode (Go to machine code, Java to JVM bytecode), they have fundamental semantic differences that require careful handling during transpilation.

## Why Java is Harder Than Other Backends

Java was the most challenging backend to implement. Here's a comparison:

| Challenge | Java | C++ | C# | Rust | JavaScript |
|-----------|------|-----|-----|------|------------|
| **Value semantics** | Objects are references - need copy constructors | Structs are values | Has `struct` (value type) | Ownership handles this | Objects are references but less strict |
| **Unsigned types** | None - must widen to signed | Has all unsigned types | Has all unsigned types | Has all unsigned types | Numbers are floats |
| **Generics + primitives** | Must use boxed types (Integer, etc.) | Templates work with any type | Generics work with value types | Generics work with any type | No static types |
| **Generic arrays** | Illegal: `new T[]` | Works fine | Works fine | Works fine | N/A |
| **Closure captures** | Must be "effectively final" - need Object[] wrapper | Captures work naturally | Captures work naturally | Explicit move/borrow | Captures work naturally |
| **Null initialization** | Everything defaults to null | Can use references/values | Nullable vs non-nullable | Option<T> is explicit | undefined/null |
| **Operator overloading** | Not supported | Supported | Supported | Supported | Not needed |

**The hardest Java-specific issues were:**

1. **Effectively final closures** - Required wrapping mutable captured variables in `Object[]` arrays
2. **Copy constructor generation** - Had to generate for every struct to preserve Go's value semantics
3. **Nested struct null initialization** - Every nested struct field needed explicit `new Type()`
4. **Boxed type juggling** - Constant conversion between `int` and `Integer` for generics

C# was probably the easiest since it has value-type structs, proper generics, and similar syntax to Go. Rust was complex but for different reasons (ownership/borrowing). C++ was straightforward due to value semantics. JavaScript was simple due to dynamic typing.

## Summary of Key Challenges

1. **Value vs Reference Semantics** - Go structs are copied on assignment; Java objects are referenced
2. **Nested Struct Initialization** - Go auto-initializes nested structs to zero values; Java leaves them null
3. **Null Struct Arguments in Constructors** - Go's partial struct init uses zero values; Java gets null
4. **Functional Interface Fields** - BiFunction/Function can't be copy-constructed
5. **Built-in Reference Types** - Object/Integer/etc. don't have copy constructors
6. **Type Mapping** - Go types to Java types (including unsigned → signed, boxed types for generics)
7. **Slice and Map Handling** - ArrayList/HashMap with helper functions
8. **For Loop Variable Scoping** - Go 1.22+ creates new variable per iteration; Java reuses
9. **Lambda Parameter Shadowing** - Go allows shadowing; Java captures outer variable
10. **Generic Array Creation** - Java doesn't allow `new ArrayList<T>[]`
11. **Multi-Return Functions** - Go returns multiple values; Java returns single value
12. **Signed/Unsigned Integers** - Go's uint8 (0-255) becomes Java's signed byte (-128 to 127); requires `& 0xFF` masking in comparisons

## Key Challenges (Detailed)

### 1. Value vs Reference Semantics

**The Problem:**
Go structs are value types - when you assign a struct to a variable or pass it to a function, a copy is made. Java objects are reference types - assignment and passing create references to the same object.

```go
// Go: This creates a copy
token := lexer.Token{Type: 1}
tokens = append(tokens, token)
token.Type = 2  // Does NOT affect the token in the slice
```

```java
// Java (naive translation): This creates a reference
Token token = new Token();
token.Type = 1;
tokens.add(token);
token.Type = 2;  // DOES affect the token in the list!
```

**The Solution:**
The emitter generates copy constructors for all struct types and wraps struct arguments to `append()` calls with copy constructor invocations:

```java
// Generated copy constructor
public Token(Token other) {
    this.Type = other.Type;
    this.Representation = other.Representation != null
        ? new ArrayList<>(other.Representation) : null;
}

// Append call with copy
tokens = SliceBuiltins.Append(tokens, new Token(token));
```

**Implementation:**
- `PostVisitGenStructInfo()` generates copy constructors for all structs
- `PreVisitCallExpr()` detects append calls with struct arguments
- `PreVisitCallExprArg()` and `PostVisitCallExprArg()` wrap struct variable references with copy constructors
- Only variable references (`*ast.Ident`) are wrapped, not constructor calls or literals (which already create new instances)

### 2. Nested Struct Initialization

**The Problem:**
In Go, nested structs are automatically initialized to their zero values. In Java, object fields default to `null`.

```go
// Go: result.OrderBy is automatically a zero-valued PgOrderByClause
var result PgSelectStatement
result.OrderBy.Fields = append(result.OrderBy.Fields, field)  // Works!
```

```java
// Java (naive): result.OrderBy is null
PgSelectStatement result = new PgSelectStatement();
result.OrderBy.Fields.add(field);  // NullPointerException!
```

**The Solution:**
The emitter initializes nested struct fields in the default constructor:

```java
public PgSelectStatement() {
    this.Distinct = false;
    this.Fields = null;  // Slices stay null (empty)
    this.From = new PgFromClause();  // Nested structs get initialized
    this.OrderBy = new PgOrderByClause();
    // ...
}
```

**Implementation:**
- `getJavaDefaultValueForStruct()` returns `new Type()` for struct types instead of `null`
- Primitives, collections, and functional interfaces still get their normal defaults
- `isJavaBuiltinReferenceType()` excludes Java built-in types like `Object` from struct initialization

### 3. Null Struct Arguments in Constructors

**The Problem:**
Go code often creates structs with only some fields set, leaving others as zero values. When translated to Java all-args constructors, unset fields become `null`.

```go
// Go: Alias is zero-valued Token, not nil
field := PgSelectField{Expression: expr}
```

```java
// Java (naive): Alias is null
PgSelectField field = new PgSelectField(expr, null);
// Later: field.Alias.Representation causes NPE
```

**The Solution:**
All-args constructors convert null struct arguments to new instances:

```java
public PgSelectField(PgExpression Expression, Token Alias) {
    this.Expression = Expression != null ? Expression : new PgExpression();
    this.Alias = Alias != null ? Alias : new Token();
}
```

**Implementation:**
- The all-args constructor generation checks if each field is a struct type
- Struct fields get null-coalescing initialization: `field != null ? field : new Type()`
- Primitives, collections, functional interfaces, and built-in reference types are assigned directly

### 4. Functional Interface Fields

**The Problem:**
Go function types become Java functional interfaces (e.g., `BiFunction`). These cannot be instantiated with `new` or have copy constructors.

```go
// Go
type Visitor struct {
    PreVisitFrom func(state any, from From) any
}
```

```java
// Java - BiFunction is an interface, not a class
public BiFunction<Object, From, Object> PreVisitFrom;

// This is WRONG:
this.PreVisitFrom = new BiFunction<...>(other.PreVisitFrom);  // Error!
```

**The Solution:**
Functional interfaces are copied by reference (which is safe since they're typically stateless lambdas):

```java
public Visitor(Visitor other) {
    this.PreVisitFrom = other.PreVisitFrom;  // Simple reference copy
}
```

**Implementation:**
- `isJavaFunctionalInterface()` detects BiFunction, Function, Consumer, Supplier, Predicate, Runnable, etc.
- Copy constructors and all-args constructors treat these as simple assignments

### 5. Built-in Reference Types

**The Problem:**
Go's `any` (interface{}) becomes Java's `Object`. Attempting to call a copy constructor on `Object` fails.

```go
// Go
type Node struct {
    Value any
}
```

```java
// Java - Object has no copy constructor
public Object Value;

// This is WRONG:
this.Value = new Object(other.Value);  // Error: Object has no such constructor
```

**The Solution:**
Built-in reference types are copied by reference:

```java
public Node(Node other) {
    this.Value = other.Value;  // Simple reference copy
}
```

**Implementation:**
- `isJavaBuiltinReferenceType()` detects Object, Integer, Long, Double, Float, Boolean, Character, Byte, Short
- These types are handled like primitives in copy constructors

### 6. Type Mapping

Go and Java have different primitive type systems:

| Go Type | Java Type | Notes |
|---------|-----------|-------|
| int8 | byte | |
| int16 | short | |
| int32, int | int | |
| int64 | long | |
| uint8 | int | Java has no unsigned types |
| uint16 | int | |
| uint32 | long | |
| uint64 | long | May lose precision for very large values |
| float32 | float | |
| float64 | double | |
| bool | boolean | |
| string | String | |
| byte | byte | |
| rune | int | Unicode code point |
| any | Object | |

**Boxed Types for Generics:**
Java generics require object types, not primitives:

| Primitive | Boxed |
|-----------|-------|
| byte | Byte |
| int | Integer |
| long | Long |
| double | Double |
| boolean | Boolean |

### 7. Slice and Map Handling

**Slices:**
Go slices become `ArrayList<T>`:
- `[]int` → `ArrayList<Integer>` (boxed for generics)
- `[]Token` → `ArrayList<Token>`
- `len(slice)` → `SliceBuiltins.Length(list)`
- `append(slice, elem)` → `SliceBuiltins.Append(list, elem)`

**Maps:**
Go maps become `HashMap<K, V>`:
- `map[string]int` → `HashMap<String, Integer>`
- `m[key]` → `m.get(key)` (with appropriate null handling)
- `m[key] = value` → `m.put(key, value)`

### 8. For Loop Variable Scoping

**The Problem:**
Go's `for` loop creates a new variable for each iteration (as of Go 1.22). Java's enhanced for loop reuses the same variable reference.

```go
// Go: Each iteration has its own 'item'
for _, item := range items {
    go func() { process(item) }()  // Each goroutine sees different item
}
```

```java
// Java (naive): All lambdas see the same 'item'
for (var item : items) {
    executor.submit(() -> process(item));  // All see final value!
}
```

**The Solution:**
Create a local copy of the loop variable inside the loop body:

```java
for (var _item : items) {
    var item = _item;  // New variable each iteration
    executor.submit(() -> process(item));
}
```

### 9. Lambda Parameter Shadowing

**The Problem:**
Go allows lambda parameters to shadow outer variables. Java closures capture the outer variable, and parameters with the same name cause confusion.

**The Solution:**
Rename lambda parameters that would shadow outer variables.

### 10. Generic Array Creation

**The Problem:**
Java doesn't allow generic array creation: `new ArrayList<T>[10]` is illegal.

**The Solution:**
Use `Object[]` and cast on access:
```java
Object[] arr = new Object[10];
// When accessing:
((ArrayList<T>)arr[i])
```

### 11. Multi-Return Functions

**The Problem:**
Go functions can return multiple values. Java methods return a single value.

**The Solution:**
Generate result classes for multi-return functions:

```go
// Go
func Parse(input string) (AST, error) { ... }
```

```java
// Java
public static class ParseResult {
    public AST _0;
    public int _1;  // error becomes int
    public ParseResult(AST _0, int _1) { ... }
}

public static ParseResult Parse(String input) { ... }

// Usage:
var result = Parse(input);
var ast = result._0;
var err = result._1;
```

## Runtime Support

The emitter generates runtime support code in the main output file:

### SliceBuiltins
Provides slice operations:
- `Append(list, element)` - Append single element
- `Append(list, elements...)` - Append multiple elements
- `Append(list, otherList)` - Append all elements from another list
- `Length(list)` - Get list length (null-safe, returns 0 for null)
- `Length(string)` - Get string length
- `Length(array)` - Get array length

### Formatter
Provides Go-style formatting:
- `Printf(format, args...)` - Print formatted output
- `Sprintf(format, args...)` - Return formatted string

### GoanyPanic
Provides panic support:
- `goPanic(message)` - Print error and exit

## File Organization

The emitter can generate code in two modes:

1. **Single File Mode:** All code in one file with runtime included
2. **Multi-File Mode:** Separate files per package, with runtime in the main file

Packages are mapped to Java classes:
- `package lexer` → `public class lexer { ... }`
- Types and functions become nested static classes and methods

## Testing

The Java backend is tested via:
1. **E2E Tests:** Compile and run example programs, compare output across backends
2. **Codegen Tests:** Verify generated code structure
3. **Manual Testing:** Complex examples like UQL parser

## Signed/Unsigned Integer Handling

Java has no unsigned integer types. This is one of the most subtle and bug-prone differences between Go and Java. Go's `uint8` (0-255) becomes Java's `byte` (-128 to 127), which requires careful handling throughout the emitter.

### The Core Problem

```go
// Go: uint8 values are 0-255
var opcode uint8 = 0xA9  // 169 in decimal

// When compared with a constant:
if opcode == 0xA9 {  // Works correctly - compares 169 == 169
    // ...
}
```

```java
// Java: byte values are -128 to 127
byte opcode = (byte)0xA9;  // Stored as -87 (sign-extended)

// Naive comparison FAILS:
if (opcode == 0xA9) {  // Compares -87 == 169 → false!
    // ...
}

// Correct comparison:
if ((opcode & 0xFF) == 0xA9) {  // Compares 169 == 169 → true!
    // ...
}
```

### Where Masking is Required

The emitter adds `& 0xFF` masking in these situations:

#### 1. Byte Comparisons with Integer Constants

When a `byte`/`uint8`/`int8` variable is compared with an integer literal or constant:

```go
// Go
if opcode == OpLDAImm {  // OpLDAImm = 0xA9
```

```java
// Java (generated)
if ((opcode & 0xFF) == OpLDAImm) {
```

**Implementation:** `PreVisitBinaryExpr()` detects when LHS is a byte type and RHS is a non-byte type (or untyped constant), then wraps LHS with `& 0xFF`.

#### 2. Byte Comparisons with Character Literals

```go
// Go
if b >= 'A' && b <= 'Z' {  // b is uint8
```

```java
// Java (generated)
if ((b & 0xFF) >= 'A' && (b & 0xFF) <= 'Z') {
```

#### 3. Byte Array/Slice Indexing

When a byte value is used as an array or slice index:

```go
// Go
result := data[index]  // index is uint8
```

```java
// Java (generated)
var result = data.get(index & 0xFF);
```

**Implementation:** `PostVisitIndexExpr()` detects byte-type indices and adds masking.

#### 4. Byte-to-Int Conversions

When explicitly converting a byte to a larger integer type:

```go
// Go
value := int(byteValue)  // byteValue is uint8
```

```java
// Java (generated)
var value = (int)(byteValue & 0xFF);
```

**Implementation:** `PreVisitCallExpr()` detects type conversion calls where the argument is a byte type and the target is a larger integer type.

#### 5. Right Shift on Bytes

Java's `>>` operator on signed bytes performs arithmetic shift (preserves sign bit). Go's `>>` on unsigned types performs logical shift:

```go
// Go: Logical shift - high bits filled with 0
result := byteValue >> 2
```

```java
// Java (generated): Must mask first to get logical shift behavior
var result = (byte)((byteValue & 0xFF) >> 2);
```

**Implementation:** `PreVisitBinaryExpr()` detects right-shift operations on byte types and adds masking.

### Places Where Masking is Already Applied

The emitter handles these cases with `& 0xFF`:

| Context | Example Go | Generated Java |
|---------|-----------|----------------|
| Comparison with constant | `b == 0xA9` | `(b & 0xFF) == 0xA9` |
| Comparison with char | `b >= 'a'` | `(b & 0xFF) >= 'a'` |
| Array index | `arr[b]` | `arr.get(b & 0xFF)` |
| Type conversion | `int(b)` | `(int)(b & 0xFF)` |
| Right shift | `b >> 2` | `(b & 0xFF) >> 2` |
| Memory read comparison | `Memory[i] == 0x80` | `(Memory.get(i) & 0xFF) == 0x80` |

### Memory and CPU Emulation Example

The MOS 6502 CPU emulator is a good example of where this matters:

```go
// Go: opcode is uint8, constants like OpLDAImm = 0xA9 (169)
var opcode uint8
c, opcode = FetchByte(c)

if opcode == OpLDAImm {  // Must compare correctly for values >= 128
    // ...
}
```

Without proper masking, opcodes with values >= 128 (like 0xA9, 0xBD, 0xE9, etc.) would never match because:
- `opcode` as Java `byte` would be negative (-87 for 0xA9)
- The constant `OpLDAImm` as Java `int` would be positive (169)
- The comparison would always fail

### Implementation Details

The masking logic is primarily in `PreVisitBinaryExpr()` in `java_emitter.go`:

1. **Detect comparison operators:** `==`, `!=`, `<`, `>`, `<=`, `>=`
2. **Check LHS type:** Use `TypesInfo.ObjectOf()` for identifiers or `TypesInfo.Types[]` for expressions
3. **Check RHS type:** Same approach
4. **Apply masking rules:**
   - If LHS is byte and RHS is non-byte (or untyped constant) → mask LHS
   - If RHS is byte and LHS is non-byte (or untyped constant) → mask RHS
   - If both are byte → no masking needed

The actual masking is emitted in `PreVisitBinaryExprOperator()`:
- Opens with `(` before the byte operand
- Closes with ` & 0xFF)` after the byte operand

### Testing Signed/Unsigned Handling

The `mos6502` examples are particularly good for testing this because:
- CPU opcodes range from 0x00 to 0xFF
- Many opcodes are >= 0x80 (128)
- Character codes for screen rendering include values >= 0x80
- Memory addresses and values span the full 0-255 range

If signed/unsigned handling is broken, these examples will fail with:
- Unrecognized opcodes (comparisons fail for values >= 128)
- Incorrect character rendering
- Wrong memory access patterns

## Known Limitations

1. **No Goroutines:** Go's goroutines are not translated (Java has different concurrency model)
2. **No Channels:** Go channels are not supported
3. **Interface Methods:** Limited support for Go interface method calls
4. **Reflection:** Go reflection is not translated
5. **Unsigned Integers:** Java has no unsigned types; large uint64 values may lose precision
6. **Pointer Arithmetic:** Not supported (rarely used in Go anyway)

## Future Improvements

1. Better error messages with source location tracking
2. Support for more Go standard library functions
3. Optimization of generated code (reduce unnecessary copies)
4. Support for Java-specific features (annotations, etc.)
