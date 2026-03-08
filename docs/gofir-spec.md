# GoFIR Specification

**Go Fragment Intermediate Representation**

Version 0.1 — February 2026

---

## Table of Contents

1. [Introduction & Motivation](#1-introduction--motivation)
2. [Design Principles](#2-design-principles)
3. [Architecture](#3-architecture)
4. [Type System](#4-type-system)
5. [Node Definitions](#5-node-definitions)
6. [Semantic Abstractions](#6-semantic-abstractions)
7. [Constraint System](#7-constraint-system)
8. [Annotation System](#8-annotation-system)
9. [Round-Trip Fidelity](#9-round-trip-fidelity)
10. [Comparison with Existing IRs](#10-comparison-with-existing-irs)
11. [LLM Synergy](#11-llm-synergy)
12. [Extensibility](#12-extensibility)

---

## 1. Introduction & Motivation

GoFIR (Go Fragment Intermediate Representation) is a typed intermediate representation designed for **source-to-source transpilation** across multiple target languages. It emerged from the goany/goany transpiler project, which compiles a Go-syntax subset to C++, C#, Rust, JavaScript, and Java.

### The Problem

Existing IRs fall into two camps:

- **Low-level IRs** (LLVM, Wasm) target machine code or bytecode. They destroy source structure—variable names, control flow shape, and programmer intent are lost.
- **Source-to-source tools** (Haxe, Nim) emit source code but provide no formal guarantee that every valid input produces valid output for every backend. Backends can fail at emission time with "unsupported feature" errors.

Neither camp offers what a multi-target source transpiler needs: an IR that **preserves source structure** while **guaranteeing universal translatability**.

### The Niche GoFIR Fills

GoFIR is a **high-level, source-preserving IR** with a **universal translatability guarantee**. If a program is representable in GoFIR, every registered backend can emit valid target code for it. No exceptions, no "unsupported node" errors.

This guarantee is achieved through a fixed constraint system that defines which programs are representable. The constraint set is the intersection of what all backends can handle. Programs that pass constraint checking are guaranteed to compile everywhere.

### Origin: goany

goany is the source language that GoFIR represents. It uses Go syntax but enforces restrictions (no goroutines, no channels, etc.) that ensure universal translatability. Some Go features (pointers, method receivers) are accepted by goany but **lowered** to common-denominator form before reaching GoFIR. The lowered form is what must satisfy the constraint set. The source language is thus a superset of what GoFIR directly represents.

---

## 2. Design Principles

GoFIR is built on seven core principles that together define its unique position among IRs.

### 2.1 Universal Translatability

> If a program is representable in GoFIR, every registered backend can emit valid code for it.

This is the strongest guarantee in the design. Backends are **total functions** from GoFIR to target source—no partiality, no fallback error paths. This is achieved by making the IR's type system and node set the intersection of what all backends support.

Unlike Haxe (where backends can fail at emission time) or Nim (where NIR can produce C that doesn't compile), GoFIR provides a compile-time guarantee: if it exists in the IR, it works everywhere.

### 2.2 Structural Preservation

> Variable names, control flow shape, function boundaries, and expression structure survive transformation.

GoFIR preserves the programmer's structural intent. A named variable in the source becomes a named variable in every backend's output. An `if/else` chain remains an `if/else` chain. Functions are not inlined or split. Expressions maintain their nesting depth.

This matters because transpiled code must be **readable and maintainable** by humans working in the target language.

### 2.3 Semantic Abstraction

> Low-level concepts are lifted to universal abstractions at the level of programmer intent.

Pointers become slice-plus-index. `malloc` becomes `make([]T, n)`. Linked lists with `*Node` become pool arrays with index fields. The abstraction level is chosen so that every target language can express it naturally.

The key insight: there are no concepts to exclude, only concepts to represent at the right abstraction level.

### 2.4 Round-Trip Fidelity

> `source → GoFIR → same-language backend ≈ source`

Output code should be semantically equivalent to and structurally recognizable as the original source. This constraint determines the correct abstraction level for each concept. If an abstraction destroys recognizability, it's too aggressive. If it preserves language-specific details, it's not abstract enough.

Round-trip fidelity is the quality metric that calibrates GoFIR's abstraction level. See [Section 9](#9-round-trip-fidelity) for detailed examples.

### 2.5 Fixed Language Definition

> One source language definition. All backends or none.

The constraint set IS the language specification. It grows slowly and deliberately. Adding a backend never shrinks what source is accepted—new backends may add constraints, but the language only becomes more precisely defined. The constraint set is the intersection of all backend requirements.

The source language may accept features beyond what GoFIR directly represents, provided each such feature has a defined **lowering** to common-denominator GoFIR. The constraint set applies to the lowered form—it is the lowered GoFIR that must satisfy all backend requirements.

### 2.6 Parametric Backend Polymorphism

> The IR carries a parametric correctness guarantee: "this program is translatable to backends B₁..Bₙ."

Like Haskell type classes, each backend provides its "instance" (constraints + annotations + emitter). The IR is the shared interface. Programs are polymorphic over backends—once validated against the constraint intersection, they compile to any backend without further checking.

### 2.7 Semantic Lowering and Lifting

> High-level constructs are lowered to common-denominator form; backends may lift them back to native constructs.

**Lowering** (desugaring) is mandatory for every accepted high-level construct. Each such construct must have a lowering rule that produces valid common-denominator GoFIR. The lowered form is what the constraint set and universal translatability guarantee apply to.

**Lifting** (re-sugaring) is optional and per-backend. If a backend has no lifting pass for a given lowered construct, the lowered form works correctly—it is already valid GoFIR. Lifting improves idiomaticity (e.g., C++ emitting real pointers instead of pool-based indexing) but is never required for correctness.

**Annotations** bridge the gap between lowering and lifting. Metadata produced during lowering preserves the original intent (e.g., "this standalone function was a method receiver"), enabling backend lifting passes to reconstruct native constructs.

**This is an evolving capability.** Features migrate from "blocked" to "lowered" as practical, semantics-preserving lowering strategies are found. Some features may remain blocked indefinitely if lowering proves impractical. The classification is not permanent—what is blocked today may become lowered tomorrow, and vice versa if a lowering turns out to be too lossy across backends.

---

## 3. Architecture

### 3.1 Pipeline Overview

```
Source Code (Go syntax, goany semantics)
    │
    ▼
┌─────────────────────────────┐
│  Frontend (Parsing + Types) │  go/parser + go/types
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────┐
│  SemaChecker (Constraints)  │  Enforces goany language rules
└─────────────┬───────────────┘  Rejects blocked features;
              │                   passes lowerable features through
              ▼
┌─────────────────────────────┐
│  Lowering Passes (desugar)  │  PointerTransformPass, etc.
└─────────────┬───────────────┘  Converts high-level constructs
              │                   to common-denominator GoFIR
              ▼
         GoFIR (common denominator — universally translatable)
              │
              ▼
┌─────────────────────────────┐
│  Analysis Passes            │  Read-only parameter detection,
└─────────────┬───────────────┘  ownership analysis, last-use
              │
              ▼
┌─────────────────────────────┐
│  Lifting Passes (re-sugar)  │  Per-backend, optional.
└─────────────┬───────────────┘  Reconstructs native constructs
              │                   from lowering annotations
              ▼
┌─────────────────────────────┐
│  GoFIR Token Generation     │  Shift/reduce on AST nodes
│  (Pre/PostVisit callbacks)  │  Produces flat token stream
└─────────────┬───────────────┘  with OptMeta annotations
              │
              ▼
┌─────────────────────────────┐
│  Optimization Passes        │  Clone elimination (Rust),
└─────────────┬───────────────┘  move optimization, ref-opt
              │
              ▼
┌─────────────────────────────┐
│  Backend Emitter            │  GoFIR tokens → target source
└─────────────┬───────────────┘
              │
              ▼
Target Source Code (C++, C#, Rust, JS, Java)
```

### 3.2 Dual-IR Design

GoFIR operates at two levels simultaneously:

1. **Tree level** — The Go AST (`go/ast`) serves as the structural backbone. The SemaChecker and analysis passes operate on this tree.

2. **Flat token level** — Backend emitters produce a flat stream of `Token` values via a shift/reduce pattern. Optimization passes rewrite this stream (e.g., inserting `.clone()` calls, changing value to reference parameters).

This dual representation allows structural analysis on the tree while enabling linear-scan optimizations on the token stream.

### 3.3 Shift/Reduce Fragment Stack

The core code generation mechanism uses a shift/reduce pattern:

1. **PreVisit** — Push a marker onto the fragment stack
2. **Process children** — Recursively visit child nodes, pushing tokens
3. **PostVisit** — Reduce: pop all tokens since the marker, transform them, and emit

```
PreVisitBinaryExpr(x + y)
├─ PushMarker("BinaryExpr")
├─ Visit left  → push token "x"
├─ Visit op    → push token "+"
├─ Visit right → push token "y"
└─ PostVisitBinaryExpr
   └─ Reduce("BinaryExpr") → ["x", "+", "y"] → emit "x + y"
```

This allows each backend to intercept and transform any subtree during reduction—for example, wrapping map keys in `Rc::new()` for Rust or converting `fmt.Sprintf` to `string.Format` for C#.

### 3.4 Lowering and Lifting Phases

**Lowering passes** run after SemaChecker and before analysis passes. They transform high-level constructs that the source language accepts into common-denominator GoFIR that all backends can handle. The lowered GoFIR is the form that the universal translatability guarantee applies to.

**Current lowering passes:**

| Pass | Input | Output | Status |
|------|-------|--------|--------|
| `PointerTransformPass` | `*T`, `&x`, pointer fields | Pool-based indexing, `RefParam` annotations | Implemented |
| `MethodReceiverLoweringPass` | `func (t T) Method()` | `func T_Method(t T)` + `DesugaredMethod` annotation | Planned |

**Lifting passes** are per-backend and optional. They run after analysis passes, before token generation. A lifting pass consumes lowering annotations and rewrites the AST back toward native constructs for backends that support them. If no lifting pass is registered, the lowered form is emitted as-is—it is already valid GoFIR.

**Example lifting behavior:**

| Lowered Form | C++ (lifted) | C#/Java (lifted) | Rust (lifted) | JS (no lifting) |
|-------------|-------------|------------------|--------------|-----------------|
| Pool-based `arr[idx]` | Real pointer `*T` | Reference field | Pool-based (same) | Pool-based (same) |
| `T_Method(t)` standalone fn | `void T::Method()` | `void Method()` in class | `impl T { fn method() }` | `T_Method(t)` (same) |

### 3.5 Proposed Standalone Architecture

The long-term architecture separates GoFIR from the Go toolchain entirely:

```
Go source ──→ Go frontend ──┐
TS source ──→ TS frontend ──┤──→ GoFIR ──→ C++ backend
Python    ──→ Py frontend ──┘           ──→ Rust backend
                                        ──→ Java backend
                                        ──→ JS backend
                                        ──→ C# backend
```

Proposed module layout:

```
gofir/
    types.go              GoFIR type system (no go/types dependency)
    nodes.go              GoFIR node definitions
    validate.go           Semantic constraints
    annotations.go        Annotation infrastructure
    walk.go               Tree traversal helpers

gofir/frontend/go/        Go-to-GoFIR converter
gofir/analysis/           Language-agnostic passes (readonly, ownership)
gofir/backend/cpp/        GoFIR → C++
gofir/backend/csharp/     GoFIR → C#
gofir/backend/rust/       GoFIR → Rust
gofir/backend/js/         GoFIR → JavaScript
gofir/backend/java/       GoFIR → Java
```

---

## 4. Type System

GoFIR defines its own type system, independent of `go/types`. The type system represents exactly the types that are universally translatable.

### 4.1 Primitive Types

| GoFIR Type | C++ | C# | Rust | JS | Java |
|------------|-----|----|------|----|------|
| `Int` (int32) | `int` | `int` | `i32` | `number` | `int` |
| `Int8` | `int8_t` | `sbyte` | `i8` | `number` | `byte` |
| `Int16` | `int16_t` | `short` | `i16` | `number` | `short` |
| `Int32` | `int32_t` | `int` | `i32` | `number` | `int` |
| `Int64` | `int64_t` | `long` | `i64` | `number` | `long` |
| `Uint8` | `uint8_t` | `byte` | `u8` | `number` | `byte` |
| `Uint16` | `uint16_t` | `ushort` | `u16` | `number` | `int` |
| `Uint32` | `uint32_t` | `uint` | `u32` | `number` | `long` |
| `Uint64` | `uint64_t` | `ulong` | `u64` | `number` | `long` |
| `Float32` | `float` | `float` | `f32` | `number` | `float` |
| `Float64` | `double` | `double` | `f64` | `number` | `double` |
| `Bool` | `bool` | `bool` | `bool` | `boolean` | `boolean` |
| `String` | `std::string` | `string` | `String` | `string` | `String` |

### 4.2 Composite Types

| GoFIR Type | C++ | C# | Rust | JS | Java |
|------------|-----|----|------|----|------|
| `Slice(T)` | `std::vector<T>` | `List<T>` | `Vec<T>` | `Array` | `ArrayList<T>` |
| `Map(K,V)` | `hmap::HashMap` | `hmap::HashMap` | `hmap::HashMap` | `Map` | `HashMap<K,V>` |
| `Struct{...}` | `struct` | `class` | `struct` | `class` | `class` |
| `Func(P...) R...` | `std::function` | `Func<>` | `Box<dyn Fn>` | `function` | `interface` |
| `Any` | `std::any` | `object` | `Box<dyn Any>` | `any` | `Object` |

### 4.3 Type Constraints

The following types are **not directly representable** in GoFIR:

- **Pointer types** (`*T`) — Lowered to `Slice(T)` + index or `RefParam` annotation (see [Section 7.2a](#72a-feature-classification-work-in-progress))
- **Channel types** (`chan T`) — No concurrency model (blocked)
- **Non-empty interfaces** — Use `Any` with type assertions or concrete types (blocked)
- **Named return types** — Return values are positional only (blocked)

### 4.4 Parameter Passing Modes

GoFIR parameters carry a `RefKind` annotation that backends map to their native mechanism:

| RefKind | Semantics | C++ | C# | Rust | JS | Java |
|---------|-----------|-----|----|------|----|------|
| `Value` | Copy/move | `T` | `T` | `T` | `T` | `T` |
| `Ref` | Read-only borrow | `const T&` | `in T` | `&T` | `T` | `T` |
| `MutRef` | Mutable borrow | `T&` | `ref T` | `&mut T` | wrapper | wrapper |

The `RefKind` is determined by the read-only analysis pass, not declared by the programmer. This keeps the source simple while letting backends generate optimal parameter passing.

---

## 5. Node Definitions

GoFIR nodes represent the structural elements of a program. Every node type must be handleable by every backend—there are no optional node types.

### 5.1 Declarations

| Node | Fields | Description |
|------|--------|-------------|
| `Package` | `Name`, `Files[]` | Top-level package |
| `FuncDecl` | `Name`, `Params[]`, `Results[]`, `Body` | Function declaration (no receiver) |
| `StructDecl` | `Name`, `Fields[]` | Struct type definition |
| `ConstDecl` | `Name`, `Type`, `Value` | Constant (explicit value, no iota) |
| `VarDecl` | `Name`, `Type`, `Value?` | Variable declaration |
| `TypeAlias` | `Name`, `Underlying` | Type alias |

### 5.2 Statements

| Node | Fields | Description |
|------|--------|-------------|
| `AssignStmt` | `Lhs[]`, `Rhs[]`, `Op` | Assignment (`=`, `:=`, `+=`, etc.) |
| `ReturnStmt` | `Results[]` | Return statement |
| `IfStmt` | `Init?`, `Cond`, `Body`, `Else?` | If statement with optional init |
| `ForStmt` | `Init?`, `Cond?`, `Post?`, `Body` | C-style for loop |
| `RangeStmt` | `Key?`, `Value?`, `X`, `Body` | Range over slice/string |
| `BlockStmt` | `Stmts[]` | Block of statements |
| `ExprStmt` | `Expr` | Expression used as statement |
| `BranchStmt` | `Kind` | `break`, `continue` |

### 5.3 Expressions

| Node | Fields | Description |
|------|--------|-------------|
| `BinaryExpr` | `Left`, `Op`, `Right` | Binary operation |
| `UnaryExpr` | `Op`, `Operand` | Unary operation |
| `CallExpr` | `Func`, `Args[]` | Function call |
| `IndexExpr` | `X`, `Index` | Slice/map indexing |
| `SelectorExpr` | `X`, `Sel` | Field access |
| `CompositeLit` | `Type`, `Elts[]` | Struct or slice literal |
| `SliceLit` | `Type`, `Elts[]` | Slice literal `[]T{...}` |
| `Ident` | `Name`, `Type` | Identifier reference |
| `BasicLit` | `Kind`, `Value` | Literal (int, float, string, etc.) |
| `FuncLit` | `Type`, `Body` | Anonymous function / closure |
| `TypeAssertExpr` | `X`, `Type` | Type assertion `x.(T)` |
| `ParenExpr` | `X` | Parenthesized expression |

### 5.4 Fragment Tags

In the current token-stream representation, each token carries a tag:

```
TagMarker  = 0    Reduction marker
TagExpr    = 1    Expression
TagStmt    = 2    Statement
TagType    = 3    Type
TagIdent   = 4    Identifier
TagLiteral = 5    Literal value
```

---

## 6. Semantic Abstractions

GoFIR represents source-language concepts at a level that every target language can express naturally. This section shows how common source patterns map to GoFIR and then to target code.

### 6.1 Pointer Abstraction

Pointers are the most significant abstraction in GoFIR. Different pointer usage patterns map to different GoFIR representations:

| Pointer Pattern | Semantic Intent | GoFIR Representation |
|----------------|-----------------|----------------------|
| `func f(x *BigStruct)` | Avoid copying | `RefParam` annotation |
| `func modify(x *int)` | Out parameter | Multi-return: `func modify(x int) int` |
| `*x = 5` through pointer | Mutation via alias | Assignment to named location |
| `type Node struct { Next *Node }` | Recursive data | Index into pool slice |
| `var p *T = nil` | Optional value | Sentinel index (-1) or optional pattern |
| `*(base + offset)` | Pointer arithmetic | `SliceAccess(slice, index)` |

#### Example: Pointer Arithmetic

**Source (C-like):**
```c
int* p = arr + 5;
*p = 42;
```

**GoFIR representation:**
```
p := SliceRef{arr, 5}
Deref{p} = 42
```

**Output (C++):**
```cpp
int p_idx = 5;
arr[p_idx] = 42;
```

**Output (Java):**
```java
int pIdx = 5;
arr.set(pIdx, 42);
```

**Output (Rust):**
```rust
let p_idx: usize = 5;
arr[p_idx] = 42;
```

Note how the named variable `p` is preserved as `p_idx` across all targets. The programmer's structural intent survives the abstraction.

#### Example: Recursive Data Structure

**Source (C-like):**
```c
struct Node {
    int value;
    Node* next;
};
```

**GoFIR representation:**
```go
type Node struct {
    Value   int
    NextIdx int   // Index into pool, -1 = null
}
// Pool: []Node used as arena
```

**Output (Rust):**
```rust
struct Node {
    value: i32,
    next_idx: i32,  // -1 = none
}
// pool: Vec<Node>
```

### 6.2 Map Operations

Maps use a custom runtime library (`hmap`) across all backends to ensure consistent semantics:

```go
// GoFIR source
m := make(map[string]int)
m["key"] = 42
v := m["key"]
```

**Output (C++):**
```cpp
auto m = hmap::HashMap<std::string, int>();
hmap::hashMapSet(m, std::string("key"), 42);
auto v = hmap::hashMapGet(m, std::string("key"));
```

**Output (Rust):**
```rust
let mut m = hmap::HashMap::new();
hmap::hash_map_set(&mut m, Rc::new("key".to_string()), 42);
let v = hmap::hash_map_get(&m, &Rc::new("key".to_string()));
```

### 6.3 Standard Library Mapping

Common standard library functions are mapped to backend-specific equivalents:

| GoFIR | C++ | C# | Rust | JS | Java |
|-------|-----|----|------|----|------|
| `fmt.Println(x)` | `println(x)` | `Console.WriteLine(x)` | `println!("{}", x)` | `console.log(x)` | `System.out.println(x)` |
| `fmt.Sprintf(f, args...)` | `string_format(f, args...)` | `String.Format(f, args)` | `format!(f, args)` | template literal | `String.format(f, args)` |
| `len(x)` | `std::size(x)` | `x.Count` | `x.len()` | `x.length` | `x.size()` |
| `append(s, e)` | `s.push_back(e)` | `s.Add(e)` | `s.push(e)` | `s.push(e)` | `s.add(e)` |
| `panic(msg)` | `goany_panic(msg)` | `throw new Exception(msg)` | `panic!(msg)` | `throw new Error(msg)` | `throw new RuntimeException(msg)` |

### 6.4 Closure Handling

Closures (function literals) are translated to each backend's native lambda mechanism:

```go
add := func(a int, b int) int {
    return a + b
}
result := add(3, 4)
```

**Output (C++):**
```cpp
auto add = [&](int a, int b) -> int {
    return a + b;
};
auto result = add(3, 4);
```

**Output (Rust):**
```rust
let add = |a: i32, b: i32| -> i32 {
    a + b
};
let result = add(3, 4);
```

**Output (Java):**
```java
BiFunction<Integer, Integer, Integer> add = (a, b) -> {
    return a + b;
};
int result = add.apply(3, 4);
```

### 6.5 Method Receiver Abstraction

Method receivers are a planned lowering target. Go methods with receivers are desugared to standalone functions with an explicit first parameter. The lowering preserves the original intent via a `DesugaredMethod` annotation, enabling backends to re-sugar into native method syntax.

**Lowering table:**

| Source | Lowered GoFIR | Annotation |
|--------|---------------|------------|
| `func (c *Counter) Inc()` | `func Counter_Inc(c *Counter)` | `DesugaredMethod{Counter, Inc, true}` |
| `func (c Counter) Value() int` | `func Counter_Value(c Counter) int` | `DesugaredMethod{Counter, Value, false}` |
| `c.Inc()` | `Counter_Inc(&c)` | |
| `c.Value()` | `Counter_Value(c)` | |

**Backend re-sugaring:**

| Backend | Re-sugared Output | Notes |
|---------|-------------------|-------|
| C++ | `void Counter::Inc()` | Member function |
| C# | `void Inc()` inside `class Counter` | Instance method |
| Java | `void inc()` inside `class Counter` | Instance method |
| Rust | `impl Counter { fn inc(&mut self) }` | Impl block |
| JS | `Counter_Inc(c)` (no lifting) | Keeps lowered form |

---

## 7. Constraint System

The constraint system is the heart of GoFIR. It defines what programs are representable—and therefore what programs are guaranteed to compile to all backends.

### 7.1 Philosophy

The constraint set IS the language specification. It is the intersection of what all backends can handle. When a new backend is added, its requirements may add constraints but never remove them. The language evolves slowly and deliberately.

This inverts the usual relationship: instead of "what can the IR express?" the question is "what can ALL backends express?" The answer defines the language.

### 7.2 Blocked Go Features

These Go features are rejected by the SemaChecker because no practical lowering exists or at least one backend cannot handle them:

| Feature | Rejected Because | Alternative |
|---------|-----------------|-------------|
| Maps (`map[K]V` literal syntax) | Limited; use `make()` with operations | `make(map[K]V)` + get/set |
| Goroutines (`go`) | No concurrency model in targets | Redesign without concurrency |
| Channels (`chan T`) | Depends on goroutines | N/A |
| `select` | Depends on channels | N/A |
| `goto` and labels | Limited support in targets | Structured control flow |
| Variadic functions (`...T`) | Semantic differences | Slice parameters |
| Non-empty interfaces | No universal equivalent | `interface{}` or concrete types |
| Struct embedding | Different semantics per target | Named fields |
| `init()` functions | No package init in targets | Explicit init from `main()` |
| Named return values | No universal equivalent | Unnamed returns |
| `iota` | Not supported in transpiler | Explicit constant values |
| `switch` (type switch) | Not universally supported | Type assertions |
| Package-level `var` | Scoping issues across targets | Local variables |

### 7.2a Feature Classification (Work in Progress)

Features in goany fall into three tiers. This classification is a living document—features migrate between tiers as the system matures, practical experience is gained, and new backends are added.

```
┌─────────┐     practical lowering found     ┌─────────┐     backend supports natively     ┌────────┐
│ Blocked │ ──────────────────────────────→  │ Lowered │ ──────────────────────────────→   │ Lifted │
│         │ ←──────────────────────────────  │         │     (optional, per backend)       │        │
└─────────┘     lowering proves impractical  └─────────┘                                   └────────┘
```

- **Blocked** — Feature is rejected by SemaChecker. No lowering exists, or lowering has proven impractical across all backends.
- **Lowered** — Feature is accepted, desugared to common-denominator GoFIR, and optionally re-sugared per backend. The lowered form is universally translatable.
- **Lifted** — A backend-specific pass reconstructs the native construct from the lowered form. Lifting is always optional; the lowered form works without it.

| Feature | Current Tier | Lowered To | Status |
|---------|-------------|-----------|--------|
| Pointers (`*T`, `&x`) | Lowered | Pool-based indexing | Implemented |
| Method receivers | Blocked | Standalone function (planned) | WIP |
| Goroutines | Blocked | — | No practical lowering known |
| Channels | Blocked | — | No practical lowering known |
| `defer` | Blocked | — | Under investigation |
| Interfaces (non-empty) | Blocked | — | Under investigation |

**Note:** This table will evolve. Features move from Blocked to Lowered when a practical, semantics-preserving lowering is found. Features may also move back to Blocked if a lowering turns out to be too lossy or impractical across all backends.

### 7.3 Backend-Specific Constraints

Beyond the universal exclusions, some constraints exist because a specific backend's semantics require them. Since GoFIR enforces the intersection, these become universal rules.

#### Rust-Driven Constraints

**String Variable Reuse After Concatenation:**
```go
// REJECTED: x is moved by + then reused
x := a + "." + b
y := x + c

// ACCEPTED: += avoids move
x := a
x += "."
x += b
y := x
y += c
```

**Same Variable Multiple Times in Expression:**
```go
// REJECTED: x moved twice
result := f(x) + g(x)

// ACCEPTED: separate statements
tmp := g(x)
result := f(x) + tmp
```

**Slice Self-Assignment:**
```go
// REJECTED: simultaneous mutable + immutable borrow
slice[i] = slice[j]

// ACCEPTED: via temporary
tmp := slice[j]
slice[i] = tmp
```

**Slice-in-Append Self-Reference:**
```go
// REJECTED: borrow conflict
slice = append(slice, slice[i])

// ACCEPTED: via temporary
tmp := slice[i]
slice = append(slice, tmp)
```

**Multiple Closures Capturing Same Variable:**
```go
// REJECTED: both closures would move/borrow x
fn1 := func() { use(x) }
fn2 := func() { use(x) }

// ACCEPTED: clone for second closure
x1 := clone(x)
fn1 := func() { use(x) }
fn2 := func() { use(x1) }
```

**Mutation After Closure Capture:**
```go
// REJECTED: closure holds reference to x
fn := func() { use(x) }
x = modify(x)

// ACCEPTED: mutate before capture
x = modify(x)
fn := func() { use(x) }
```

#### C#-Driven Constraints

**Variable Shadowing Across Sibling Scopes:**
```go
// REJECTED: C# CS0136 - can't declare same name in sibling scopes
if a {
    x := 1
}
if b {
    x := 2
}

// ACCEPTED: hoist declaration
var x int
if a {
    x = 1
} else {
    x = 2
}
```

#### C++-Driven Constraints

**Struct Field Initialization Order:**
```go
// REJECTED: C++ designated initializers require declaration order
type S struct { A int; B int }
s := S{B: 2, A: 1}

// ACCEPTED: match declaration order
s := S{A: 1, B: 2}
```

#### Cross-Backend Constraints

**No Nil Comparisons:**
```go
// REJECTED: no universal null for slices
if x == nil { ... }

// ACCEPTED: check length
if len(x) == 0 { ... }
```

**No Mutation During Iteration:**
```go
// REJECTED: borrow conflict in Rust
for i := 0; i < len(items); i++ {
    items = append(items, newItem)
}

// ACCEPTED: collect changes, apply after loop
var toAdd []T
for i := 0; i < len(items); i++ {
    toAdd = append(toAdd, newItem)
}
items = append(items, toAdd...)
```

### 7.4 Constraint Summary Table

| Constraint | Driven By | Category |
|-----------|-----------|----------|
| String variable reuse after `+` | Rust move semantics | Ownership |
| Variable shadowing across scopes | C# CS0136 | Scoping |
| Struct field init order | C++ designated initializers | Initialization |
| Slice self-assignment `s[i] = s[j]` | Rust borrow checker | Borrowing |
| Multiple closures capturing same var | Rust ownership | Ownership |
| Mutation after closure capture | Rust ownership | Ownership |
| Same var multiple times in expression | Rust move semantics | Ownership |
| Collection mutation during iteration | Rust borrow checker | Borrowing |
| No nil comparisons | Multi-target (no universal null) | Type safety |
| No goroutines/channels | Multi-target (no universal concurrency) | Concurrency |
| Pointers (`*T`, `&x`) | Multi-target (memory models differ) | Lowered (pool-based indexing) |
| Method receivers | Multi-target (class models differ) | Blocked (lowering planned) |

### 7.5 Structural Enforcement

In the standalone GoFIR design, many constraints are enforced structurally by making invalid programs **unrepresentable**:

| Constraint | Current (SemaChecker) | Standalone GoFIR | Tier |
|-----------|----------------------|------------------|------|
| Pointers (`*T`) | Lowering pass | Pool-based indexing in type system | Lowered |
| No channels | Runtime check | No `ChanType` in type system | Blocked |
| Method receivers | Runtime check (planned: lowering) | `FuncDecl` has no `Recv` field | Blocked (WIP) |
| No variadic params | Runtime check | `Param` has no `Ellipsis` flag | Blocked |
| No named returns | Runtime check | Results are `[]Type`, not named | Blocked |
| No struct embedding | Runtime check | `Field` requires `Name` | Blocked |
| No nil comparison | Runtime check | Requires validation pass | Blocked |
| No string reuse after `+` | Runtime check | Requires validation pass | Blocked |

---

## 8. Annotation System

Annotations are per-backend metadata attached to GoFIR nodes. They allow backend-specific information (ownership, borrowing, type mapping) without polluting the core IR.

### 8.1 Design

```
GoFIR Node
  ├─ Core fields (universal)
  └─ Annotations map[string]any (per-backend, extensible)
```

Annotation passes run after constraint checking and before emission. Each backend registers the passes it needs. Passes are language-agnostic analyzers that produce backend-consumable metadata.

### 8.2 Optimization Metadata (OptMeta)

In the current implementation, each token carries an optional `OptMeta` structure:

| Field | Purpose |
|-------|---------|
| `Kind` | Optimization category (Clone, FuncParam, etc.) |
| `VarName` | Variable name for move/clone decisions |
| `ParamIndex` | Parameter position in function signature |
| `FuncKey` | Qualified function key (`pkg.FuncName`) |
| `IsInsideClosure` | Whether expression is inside a lambda |
| `TypeStr` | Type string for signature modifications |

### 8.2a Lowering Annotations

Lowering passes attach annotations that preserve the original high-level intent. These annotations are consumed by optional lifting passes to reconstruct native constructs.

**DesugaredPointer:**

| Field | Type | Description |
|-------|------|-------------|
| `ElemType` | string | The pointed-to type (e.g., `Node`, `int`) |
| `PoolName` | string | Name of the pool slice (e.g., `nodePool`) |
| `Pattern` | enum | `ParamPtr` — pointer parameter, `LocalPtr` — local pointer variable, `FieldPtr` — struct field pointer, `IndexPtr` — pointer via index into pool |

**DesugaredMethod:**

| Field | Type | Description |
|-------|------|-------------|
| `ReceiverType` | string | The type the method was defined on (e.g., `Counter`) |
| `MethodName` | string | The original method name (e.g., `Inc`) |
| `IsPointerRecv` | bool | Whether the original receiver was a pointer (`*T`) |

### 8.3 Analysis Passes

**Read-Only Parameter Analysis:**

Determines which function parameters are never mutated, enabling optimized passing:

```
Input:  func process(data []Token, name string) Node
Analysis:
  - data: mutated via indexing → MutRef
  - name: never mutated, never returned → Ref

Output annotations:
  - data.RefKind = MutRef  → Rust: &mut Vec<Token>,  C#: ref List<Token>
  - name.RefKind = Ref     → Rust: &String,           C#: in string
```

The analysis tracks:
- **Mutated variables** — any assignment, including slice element writes
- **Directly mutated variables** — field reassignment, whole-variable assignment
- **Returned variables** — cannot be read-only if returned
- **Assigned-from variables** — cannot be ref if value is captured

**Move Optimization (Rust):**

Eliminates unnecessary `.clone()` calls when a variable is being reassigned from a call result:

```rust
// Before optimization (3 clones):
c = doADC(c.clone(), ReadIndirectX(c.clone(), zp));

// After move optimization (1 clone):
c = doADC(c, ReadIndirectX(c.clone(), zp));
```

Conditions for move optimization:
1. Variable must be LHS of the assignment containing the call
2. Variable must NOT appear multiple times in the outermost call's arguments
3. Expression must NOT be inside a closure

### 8.4 Backend Profiles

Each backend declares its annotation requirements:

```
RustProfile:
  Constraints: [no-string-reuse, no-slice-self-assign, no-multi-capture, ...]
  Annotations: [ownership, move-opt, ref-opt]

CSharpProfile:
  Constraints: [no-variable-shadowing, field-init-order]
  Annotations: [ref-opt, mut-ref-opt]

CppProfile:
  Constraints: [field-init-order]
  Annotations: [ref-opt, move-opt]

JavaProfile:
  Constraints: []
  Annotations: [boxed-types]

JSProfile:
  Constraints: []
  Annotations: []
```

---

## 9. Round-Trip Fidelity

Round-trip fidelity is the design constraint that calibrates GoFIR's abstraction level. It answers: "How do we know the abstraction is right?"

### 9.1 Definition

A round-trip is: `source → GoFIR → same-language backend → output`. The output should be:

- **Semantically equivalent** to the input
- **Structurally recognizable** — a programmer reading the output should recognize their original code

### 9.2 What Must Be Preserved

| Source Structure | GoFIR Must Preserve | Must Not Lower To |
|-----------------|--------------------|--------------------|
| Named variable `int p = ...` | Named variable `p` | Inline the value everywhere |
| Loop with index `i++` | Loop with index `i++` | Restructured while loop |
| Argument passed to function | Argument at same call site | Inlined at call site |
| Local variable names | Same names | Renamed temporaries |
| Control flow shape (if/else/for) | Same structure | Flattened or restructured |
| Expression nesting depth | Same depth | Decomposed into temps |
| Function boundaries | Same functions | Inlined or split |

### 9.3 Quality Metric

```
diff(original, roundtrip) → structural distance

Perfect:     Only cosmetic differences (whitespace, formatting)
Acceptable:  Variable names preserved, control flow preserved,
             1-to-1 correspondence between statements
Failing:     Restructured logic, eliminated variables,
             different control flow shape
```

### 9.4 Examples

#### Good Round-Trip (Structure Preserved)

**Input (Go/goany):**
```go
func sum(values []int) int {
    total := 0
    for i := 0; i < len(values); i++ {
        total += values[i]
    }
    return total
}
```

**Output via Go backend (hypothetical):**
```go
func sum(values []int) int {
    total := 0
    for i := 0; i < len(values); i++ {
        total += values[i]
    }
    return total
}
```

Structural distance: zero. Perfect round-trip.

#### Good Round-Trip (Cross-Language, Structure Preserved)

**Input (Go/goany):**
```go
func findFirst(tokens []Token, typ int) int {
    for i := 0; i < len(tokens); i++ {
        if tokens[i].Type == typ {
            return i
        }
    }
    return -1
}
```

**Output (Rust):**
```rust
fn find_first(tokens: &Vec<Token>, typ: i32) -> i32 {
    for i in 0..tokens.len() {
        if tokens[i].type_field == typ {
            return i as i32;
        }
    }
    return -1;
}
```

Variable names preserved (snake_case convention). Control flow identical. Function boundary intact. A Go programmer can read this Rust and recognize their code.

#### Bad Round-Trip (Over-Abstraction)

**Input (hypothetical C with pointer):**
```c
int* p = arr + 5;
*p = 42;
```

**Bad output (pointer variable eliminated):**
```c
arr[5] = 42;
```

Semantically equivalent but structurally different. The variable `p` is gone. The programmer wouldn't recognize their code. This means the abstraction was too aggressive—GoFIR should preserve `p` as `p_idx`.

**Good output (structure preserved):**
```c
int p_idx = 5;
arr[p_idx] = 42;
```

The variable survives. The intent is clear.

---

## 10. Comparison with Existing IRs

### 10.1 Feature Matrix

| Feature | GoFIR | LLVM | MLIR | Haxe | Wasm | Babelfish |
|---------|-------|------|------|------|------|-----------|
| Source-agnostic IR | Planned | Yes | Yes | No | Yes | Yes |
| Multi-target source output | Yes | No | No | Yes | No | No |
| Translatability guarantee | Yes | Yes* | No | No | Yes* | N/A |
| Structural preservation | Yes | No | Partial | Partial | No | Yes |
| Round-trip fidelity | Yes | No | No | No | No | N/A |
| Semantic lifting | Yes | No | Partial | N/A | No | No |

\* LLVM/Wasm guarantee for machine targets, not source targets.

### 10.2 Detailed Comparisons

**LLVM IR:**
Source-agnostic with a strong guarantee—but targets machine code. Round-trip to C exists (via decompilation) but destroys structure completely. Nobody reads LLVM→C output and recognizes their program. GoFIR operates at a fundamentally higher abstraction level.

**MLIR:**
Source-agnostic with arbitrary dialects. Powerful and extensible. But not designed for source-to-source transpilation with multi-target source output. MLIR dialects lower toward hardware, not toward other programming languages.

**Haxe:**
Multi-target source output from a single source language—the closest existing system to GoFIR. But: (1) backends can fail at emission time with "target doesn't support this" errors, (2) no formal translatability guarantee, (3) no round-trip concept, (4) not a reusable IR library.

**WebAssembly:**
Source-agnostic compilation target with a strong guarantee. But Wasm is bytecode—it outputs a binary format, not source code. Wasm→source decompilation loses all structure.

**Babelfish UAST:**
Source-agnostic AST with multiple language parsers—designed for code analysis (linting, metrics). But read-only: no backends, no code generation, no translatability guarantee.

**Decompilers (Ghidra, IDA):**
Attempt structural preservation and round-trip (binary→source). But single target language, no translatability guarantee, and the round-trip is lossy (variable names lost, control flow restructured).

**Model-Driven Architecture (MDA/MDE):**
Has a round-trip concept (model↔code). But operates at model level (UML diagrams), not source level. Typically single target language.

### 10.3 What's Genuinely Novel

GoFIR is an IR where the **round-trip property** is the design constraint that determines the **abstraction level**, AND the IR guarantees **universal translatability** to multiple source language targets.

These two requirements pull in opposite directions:
- Round-trip wants the IR **close to source** (preserve structure)
- Universal translatability wants the IR **abstract** (hide language differences)

Finding the level that satisfies both is GoFIR's core contribution. No existing system attempts both simultaneously.

---

## 11. LLM Synergy

GoFIR is designed to complement LLMs, not compete with them. Each has strengths the other lacks.

### 11.1 What LLMs Do Well

- Translate small, self-contained programs (under ~200 lines) with reasonable quality
- Produce idiomatic-sounding output that reads naturally in the target language
- Handle ambiguous or incomplete specifications through probabilistic reasoning

### 11.2 Where LLMs Fundamentally Fail

**Consistency across a codebase:**
Translate 50 files and the LLM makes different type mapping decisions in file 17 vs file 3. No shared schema, no contract. A transpiler enforces uniform decisions.

**Correctness guarantee:**
An LLM gives probable output. A transpiler gives proven output. Example: byte signedness—an LLM sometimes produces `(long)(data.get(0) & 0xFF)` (correct unsigned extension) and sometimes `(long)(data.get(0))` (incorrect signed extension) depending on context window contents.

**Reproducibility:**
Same input, different output each run. A transpiler is deterministic.

**Incremental updates:**
Change one function in Go, and a transpiler regenerates only affected output. An LLM re-translates the whole file and changes 15 unrelated lines.

### 11.3 The Hybrid Architecture

GoFIR enables three complementary uses with LLMs:

**1. Validation Layer**

Parse both source and LLM-generated output into GoFIR. Compare the two representations for semantic equivalence. GoFIR becomes a semantic diff tool.

```
Go source → Go frontend → GoFIR_A
LLM-generated Rust → Rust frontend → GoFIR_B
Compare GoFIR_A ≈ GoFIR_B → semantic equivalence check
```

**2. Training Data**

A corpus of `(GoFIR, target code)` pairs is high-quality training data for translation models. The IR provides ground truth alignment that's missing from current LLM training on raw source pairs.

**3. Hybrid Backend**

Generate correct-but-mechanical code from GoFIR, then use an LLM to polish it with the constraint: "make this idiomatic, don't change semantics." GoFIR's round-trip test validates the LLM didn't break anything.

```
GoFIR → deterministic backend → correct Rust (mechanical)
     → LLM polish pass → idiomatic Rust (natural)
     → round-trip validation → confirmed correct
```

### 11.4 Summary

LLMs made one-off, small-scale code translation easy—covering roughly 80% of transpiler use cases. The remaining 20%—correctness guarantees, consistency at scale, reproducibility, incremental updates, CI/CD integration—is where transpilers are irreplaceable. GoFIR complements LLMs; they don't compete.

---

## 12. Extensibility

### 12.1 Adding a New Backend

Adding a new target language requires:

1. **Define a BackendProfile** — declare which constraints apply and which annotation passes are needed
2. **Implement the Emitter interface** — handle every GoFIR node type (the compiler enforces completeness)
3. **Add type mappings** — map GoFIR types to target language types
4. **Add runtime library** — implement `hmap`, `fmt` equivalents in the target language

The backend must be a **total function**: every valid GoFIR node must produce valid output. This is what makes the universal translatability guarantee work.

If the new backend requires a constraint that doesn't exist yet (e.g., "no nested closures"), that constraint is added to the constraint library and becomes available for all backends. The goany language definition may need to adopt it if it's required.

### 12.2 Adding a New Frontend

Adding a new source language requires:

1. **Parse the source language** into its native AST
2. **Translate to GoFIR nodes** — map source constructs to GoFIR equivalents
3. **Apply semantic lifting** — convert language-specific constructs (pointers, classes, etc.) to GoFIR abstractions

The frontend must produce valid GoFIR—the constraint checker validates the output. If a source construct cannot be represented in GoFIR, the frontend must either lift it to an equivalent GoFIR pattern or reject it with a clear error message.

### 12.3 Adding a New Analysis Pass

Analysis passes are language-agnostic transformations on GoFIR:

1. **Define the analysis** — what information does it compute?
2. **Implement the pass** — walk GoFIR, compute results, attach as annotations
3. **Register with backends** — backends that benefit from the analysis add it to their profile

Example analysis passes:
- Read-only parameter detection (enables `const&`, `&T`, `in T`)
- Last-use detection (enables move instead of clone)
- Escape analysis (determines heap vs stack allocation hints)
- Dead code elimination (removes unreachable branches)

### 12.4 Adding a Lifting Pass

Lifting passes are per-backend, optional transformations that reconstruct native constructs from lowered GoFIR. Adding a lifting pass follows this pattern:

1. **Identify the lowering annotation to consume** — e.g., `DesugaredPointer` or `DesugaredMethod`. The annotation contains the original high-level intent.
2. **Implement the AST rewrite** — transform the lowered GoFIR nodes back toward the native construct for the target language. For example, rewrite pool-based indexing back to real pointer dereferences for C++.
3. **Register with the backend** — add the lifting pass to the backend's profile so it runs after analysis passes, before token generation.
4. **Test with and without** — lifting improves idiomaticity but is never required for correctness. The lowered form must always produce valid output. Both paths (with and without lifting) must be tested.

This architecture ensures that adding a lifting pass is always safe—it's a pure improvement in output quality, not a correctness requirement. If a lifting pass has a bug, disabling it produces correct (if less idiomatic) output.

**Note:** As the lowering/lifting architecture matures, new lifting passes will be added for new backends and new lowered features. The set of lifting passes is expected to grow over time as more features move from Blocked to Lowered.

### 12.5 Evolution Philosophy

GoFIR evolves conservatively:

- **New backends** may add constraints but never remove them
- **New frontends** produce the same GoFIR nodes—they don't extend the IR
- **New analysis passes** add annotations without changing core nodes
- **The constraint set grows monotonically** — once a program is valid, it stays valid
- **Breaking changes** require a new major version

This ensures that code written to GoFIR today will continue to compile to all backends tomorrow.

---

## Appendix A: Current Backend Support

| Backend | Status | Optimization Passes |
|---------|--------|-------------------|
| C++ | Production | Move optimization, reference optimization |
| C# | Production | Reference optimization, mutable-ref optimization |
| Rust | Production | Move optimization, reference optimization, clone elimination |
| JavaScript | Production | None (all objects are references) |
| Java | Production | Boxed type handling |

## Appendix B: Glossary

| Term | Definition |
|------|-----------|
| **GoFIR** | Go Fragment Intermediate Representation |
| **goany** | The source language defined by GoFIR's constraint set |
| **SemaChecker** | Semantic analysis pass that enforces constraints |
| **Fragment Stack** | Shift/reduce mechanism for token generation |
| **OptMeta** | Per-token optimization metadata |
| **RefKind** | Parameter passing mode (Value, Ref, MutRef) |
| **BackendProfile** | Declaration of a backend's constraints and annotation needs |
| **Round-trip** | source → GoFIR → same-language backend → output |
| **Structural distance** | Measure of how much structure changed in a round-trip |
| **Lowering** | Transforming a high-level construct to common-denominator GoFIR (mandatory) |
| **Lifting** | Reconstructing a native construct from lowered GoFIR for a specific backend (optional) |
| **Desugar** | Synonym for lowering — removing syntactic sugar to reveal underlying form |
| **Re-sugar** | Synonym for lifting — restoring syntactic sugar for a specific backend |
| **DesugaredPointer** | Lowering annotation preserving pointer intent (ElemType, PoolName, Pattern) |
| **DesugaredMethod** | Lowering annotation preserving method receiver intent (ReceiverType, MethodName, IsPointerRecv) |
