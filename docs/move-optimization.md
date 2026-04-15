# Move Optimization for Struct Arguments in Rust and C++ Emitters

## Problem Statement

The GoAny transpiler generates Rust and C++ code from Go sources. Go's value semantics mean structs are implicitly copied when passed to functions. In Go, this is handled by the runtime with cheap stack copies. However, when transpiled:

- **Rust**: The emitter adds `.clone()` to every struct argument to preserve Go's copy semantics, since Rust moves values by default.
- **C++**: Structs are passed by value (implicit copy), which triggers the copy constructor.

For small structs this is negligible. For structs containing heap-allocated data (e.g., `Vec<u8>`, `std::vector`), each copy triggers a deep clone of the heap data.

### Motivating Example: MOS 6502 CPU Emulator

The C64 emulator's CPU struct contains a 64KB memory array:

```go
type CPU struct {
    A      uint8     // Accumulator
    X      uint8     // X register
    Y      uint8     // Y register
    SP     uint8     // Stack pointer
    PC     int       // Program counter
    Status uint8     // Status register
    Memory []uint8   // 64KB memory  <-- expensive to copy
    Halted bool
    Cycles int
}
```

The emulator uses a functional style where every CPU operation takes a CPU by value and returns a new one:

```go
func Step(c CPU) CPU { ... }
func FetchByte(c CPU) (CPU, uint8) { ... }
func SetZN(c CPU, value uint8) CPU { ... }
```

A single `Step` call produces a chain of 3-5 sub-calls, each requiring the CPU struct. The CLR command executes ~1001 instructions, generating **~300MB of unnecessary memcpy** per frame in the unoptimized output.

---

## Baseline Rules (Before Optimization)

### Rust Emitter: Unconditional `.clone()`

**Rule**: Every struct argument to a function call gets `.clone()` appended.

```
Go source:      c = Step(c)
Rust output:    c = Step(c.clone());
```

```
Go source:      c = SetZN(c, c.A)
Rust output:    c = SetZN(c.clone(), c.A);
```

```
Go source:      c, value = FetchByte(c)
Rust output:    let (_c, _value) = FetchByte(c.clone());
```

**Rationale**: In Go, passing a struct by value creates an independent copy. Rust moves values by default, which would invalidate the original variable. Adding `.clone()` preserves Go's value semantics at the cost of a deep copy.

### Rust Emitter: Unconditional Multi-Value Return `.clone()`

**Rule**: In functions returning multiple values as a tuple, the first result always gets `.clone()` if it is a struct.

```
Go source:      return c, value
Rust output:    return (c.clone(), value);
```

**Rationale**: Both `c` and `value` might reference the same data (e.g., `return c, c.Memory[addr]`). Cloning the first result prevents "borrow of moved value" errors.

### C++ Emitter: Implicit Copy

**Rule**: Structs are passed by value with no `std::move()`. The C++ compiler invokes the copy constructor.

```
Go source:      c = Step(c)
C++ output:     c = Step(c);           // implicit 65KB copy
```

```
Go source:      return c, value
C++ output:     return std::make_tuple(c, value);  // c copied into tuple
```

**Rationale**: C++ value semantics match Go's semantics. However, when the source variable is immediately overwritten (`c = Step(c)`), the copy is wasteful since the old value is discarded.

---

## Architecture: Three-Stage IR Pipeline

All move and reference optimizations are implemented via a three-stage IR pipeline. The emitter generates conservative code as an IR tree (`IRNode` with `Children`, `Content`, `OptMeta`), and two IR passes progressively optimize it.

```
Emitter (conservative .clone()) → CloneMovePass (remove clones / add moves) → RefOptPass (add references)
```

### Stage 1: Emitter (rust_expr.go, rust_stmt.go, rust_opt.go)

The Rust emitter generates code with `.clone()` on all non-Copy call arguments and multi-return values. Each argument is collected as an `IRNode` tree using `IRForestBuilder` (scoped token collection). The emitter annotates nodes with `OptMeta` flags indicating optimization opportunities:

- `CanTransfer` -- variable can be moved (last use before reassignment)
- `IsReassignedSource` -- source location matches the assignment target (for `std::mem::take`)
- `IsExtractedArg` -- argument extracted to a temp variable to avoid aliasing
- `IsElementCopy` -- collection element already copied (no double-clone)
- `IsOwnedValue` -- expression already produces a fresh owned value

**Skipped cases** (no `.clone()` added):
- Copy types (integers, booleans, floats) -- cheap bitwise copy
- String literals -- already produce an owned `String` via `.to_string()`
- Composite literals -- already produce a fresh owned value
- Function call results -- already return an owned value
- Function literals (closures) -- passed by value
- Vec element accesses -- already produce an owned copy via index clone

### Stage 2: CloneMovePass (clone_move_pass.go)

The `CloneMovePass` is an `IRPass` that walks the IR tree bottom-up, reading `OptMeta` annotations:

| Flag | Rust Transformation | C++ Transformation |
|---|---|---|
| `CanTransfer` | Remove `.clone()` (move) | Wrap with `std::move()` |
| `IsReassignedSource` | Replace with `std::mem::take(&mut expr)` | -- |
| `IsExtractedArg` | Extract to temp variable before assignment | -- |
| Builtin callee (len, delete, etc.) | Remove `.clone()` | -- |
| No flag | Clone stays (conservative) | No change |

For `IsExtractedArg`, the pass saves the original arg tree and replaces it with the temp variable name. Then in `transformAssignment`, it builds `let __mv0: Type = <saved_tree>;` bindings before the assignment using `MultiNode` splicing.

### Stage 3: RefOptPass (ref_opt_pass.go)

The `RefOptPass` is an `IRPass` that transforms function parameters and call arguments to use reference semantics. It runs in two phases:

1. **Collect**: Walk the tree to find all `OptFuncParam` annotations with `IsReadOnly` / `IsMutRef` flags
2. **Transform**: For each `OptCallArg` whose callee has a read-only parameter at that index, strip `.clone()` and add `&` prefix

| Language | Param Transform | Call-site Transform |
|---|---|---|
| Rust | `mut name: T` → `name: &T` | `x.clone()` → `&x` |
| C++ | `Type name` → `const Type& name` | (unchanged) |
| C# | `Type name` → `in Type name` | `x` → `in x` |

See also: `docs/clone-optimization.md` for a concise pipeline overview.

---

## Optimization 1: Move Detection for Reassigned Variables

**Pattern**: `variable = Function(variable, ...)`

When a struct variable appears on both the left-hand side and as a function argument, the old value is being consumed and replaced. This is semantically a **move**, not a copy.

### Conditions for Safe Move

All conditions must be met:

1. **Assignment context**: The variable appears on the LHS of the current assignment statement.
2. **Single reference**: The variable appears only once across all arguments of the call (count <= 1).
3. **Not inside a closure**: The code is not inside a function literal (`funcLitDepth == 0`). Rust's `FnMut` closures cannot move captured variables.

### Rust Implementation

When conditions are met, the emitter sets `CanTransfer: true` on the `OptMeta` of the argument node (which has `.clone()` appended conservatively). The `CloneMovePass` then removes the `.clone()` suffix, allowing Rust's move semantics to transfer ownership:

```
Go source:      c = Step(c)

Before:         c = Step(c.clone());      // 65KB deep copy
After:          c = Step(c);              // zero-cost move
```

Multi-value return variant:

```
Go source:      c, value = FetchByte(c)

Before:         let (_c, _value) = FetchByte(c.clone());
After:          let (_c, _value) = FetchByte(c);
```

### C++ Implementation

When conditions are met, the emitter sets `CanTransfer: true`. The `CloneMovePass` wraps the argument with `std::move()`:

```
Go source:      c = Step(c)

Before:         c = Step(c);              // implicit 65KB copy
After:          c = Step(std::move(c));   // zero-cost move
```

### Why Multiple References Block the Optimization

```go
c = SetZN(c, c.A)
```

Here `c` appears twice in the arguments: as the first argument (`c`) and inside the second argument (`c.A`). In Rust, moving `c` in the first argument would invalidate `c.A` in the second. The clone must be preserved:

```rust
c = SetZN(c.clone(), c.A);   // clone required: c used twice
```

### Closure Safety

Inside closures, Rust's borrow checker prevents moving captured variables:

```rust
graphics::RunLoop(w.clone(), |w: graphics::Window| {
    // c is captured by the FnMut closure
    c = scrollScreenUp(c.clone());  // MUST clone: can't move out of closure
});
```

The `funcLitDepth` counter tracks closure nesting to disable the optimization inside function literals.

### Impact

Eliminated **~192 `.clone()` calls** from the CPU emulator code (319 to 127).

---

## Optimization 2: Temporary Variable Extraction for Field Accesses

**Pattern**: `variable = Function(variable, variable.Field)`

When a struct variable appears multiple times in call arguments (blocking Optimization 1), but some references are simple field accesses producing Copy-type values, we can **extract those field accesses into temporary variables** before the call.

### Mechanism

Before the assignment, emit `let` bindings that capture the field values. Then replace the corresponding arguments with the temp variable names. This reduces the struct's reference count to 1, enabling the move.

```
Go source:      c = SetZN(c, c.A)

Before:         c = SetZN(c.clone(), c.A);

After:          let __mv0: u8 = c.A;
                c = SetZN(c, __mv0);
```

The field `c.A` is read into `__mv0` while `c` is still alive. Then `c` is moved (not cloned) into `SetZN`. The temporary holds the extracted Copy value.

### Conditions for Extraction

An argument is extractable when:

1. **References the struct**: The argument expression contains the struct variable being moved.
2. **Copy-type result**: The expression evaluates to a Rust Copy type (bool, integers, floats).
3. **Convertible to Rust code**: The `exprToRustCodeOpt` function (`rust_opt.go`) can generate a valid Rust expression for the argument (handles identifiers, selectors, basic literals, index expressions, binary expressions, paren expressions, and type conversions).
4. **Not inside a closure**: `funcLitDepth == 0`.

### Multi-Argument Extraction

Multiple arguments can be extracted in a single call:

```
Go source:      c = doCMP(c, c.A, c.Memory[int(addr)])

Before:         c = doCMP(c.clone(), c.A, c.Memory[(addr as i32) as usize]);

After:          let __mv0: u8 = c.A;
                let __mv1: u8 = c.Memory[(addr as i32) as usize];
                c = doCMP(c, __mv0, __mv1);
```

### Modified Reference Counting

After extracting arguments, the optimization recalculates the struct's reference count by subtracting the identifiers found in extracted expressions. If the modified count drops to <= 1, `canMoveArg` returns true.

For `c = SetZN(c, c.A)`:
- Original count: `c` appears 2 times (arg 0: `c`, arg 1: `c.A`)
- After extracting `c.A`: subtract 1 from count of `c`
- Modified count: 1 (only arg 0: `c`)
- `canMoveArg("c")` returns true, `.clone()` omitted

### IR Tree Implementation

The extraction uses the `IRForestBuilder` scoped collection and `OptMeta` annotations:

1. `PreVisitAssignStmt` calls `analyzeMoveOptExtraction` (`rust_opt.go`), which scans the RHS call's arguments to find field accesses on the struct being moved, creates `TempExtraction` records, and computes modified reference counts.
2. `PostVisitCallExprArg` checks if the current arg index matches a `TempExtraction` slot. If so, it collects the emitted IR forest for the arg, attaches `OptMeta{IsExtractedArg: true, ExtractedName: "__mv0"}`, and adds it to the tree.
3. `PostVisitAssignStmt` attaches `OptMeta{Kind: OptAssignment, TempExtractions: [...]}` to the assignment node.
4. `CloneMovePass.transformCallArg` saves the original arg tree and replaces the node content with the temp variable name.
5. `CloneMovePass.transformAssignment` builds `let __mv0: Type = <saved_tree>;` binding nodes and returns a `MultiNode` that splices them before the assignment.

### Nested Call Extraction Safety

When an extracted argument is itself a function call (e.g., `c = doADC(c, ReadIndirectX(c, zp))`), the inner call's arguments must NOT have their `.clone()` removed by `canMoveArg`. Otherwise, moving `c` inside `ReadIndirectX` would invalidate it before the outer `doADC` call.

The `moveOptInsideExtractedCall` flag (`rust_opt.go`) handles this:

1. `PreVisitCallExprArg` sets `moveOptInsideExtractedCall = true` when entering an arg that matches a `TempExtraction` slot and is a `*ast.CallExpr`.
2. `canMoveArg` returns `false` when this flag is set, preserving `.clone()` on inner args.
3. `PostVisitCallExprArg` clears the flag when `callExprArgDepth` returns to 0.

```
Go source:      c = doADC(c, ReadIndirectX(c, zp))

Without fix:    let __mv0: u8 = ReadIndirectX(c, zp);   // ERROR: c moved
                c = doADC(c, __mv0);

With fix:       let __mv0: u8 = ReadIndirectX(c.clone(), zp);  // clone preserved
                c = doADC(c, __mv0);
```

### Impact

Eliminated **44 additional `.clone()` calls** (127 to 83), generating 161 temporary variable bindings.

---

## Optimization 3: Type Conversion Support in Expression Extraction

**Pattern**: `variable = Function(variable, variable.Field[TypeConversion(expr)])`

Go type conversions like `int(addr)` and `uint8(c.A & val)` are represented as `CallExpr` nodes in the Go AST. The initial `exprToRustCodeOpt` function (`rust_opt.go`) could not handle `CallExpr`, causing extraction to fail for expressions containing type casts.

### Before

Expressions with type conversions returned `""` from `exprToRustCodeOpt`, blocking extraction:

```
Go source:      c = doADC(c, c.Memory[int(addr)])

Could not extract c.Memory[int(addr)] because int(addr) is a CallExpr.
Result:         c = doADC(c.clone(), c.Memory[(addr as i32) as usize]);
```

### After

`exprToRustCodeOpt` now detects single-argument `CallExpr` where the function is a Go built-in type name (found in `rustTypesMap`), and emits a Rust `as` cast:

```
Go type conversion:    int(addr)
Rust output:           (addr as i32)

Go type conversion:    uint8(c.A & val)
Rust output:           ((c.A & val) as u8)
```

Full example:

```
Go source:      c = doADC(c, c.Memory[int(addr)])

After:          let __mv0: u8 = c.Memory[(addr as i32) as usize];
                c = doADC(c, __mv0);
```

### Cast Hint Propagation

When processing expressions inside a type conversion, a `castHint` parameter is propagated through the expression tree. If an identifier is a constant whose Rust type differs from the cast hint, it is explicitly cast:

```
Go source:      c = PushByte(c, uint8(c.Status | FlagB | 0x20))
```

`FlagB` is an untyped Go constant that becomes `i32` in Rust, but `c.Status` is `u8`. Without the cast hint, this would produce `c.Status | FlagB` which is `u8 | i32` (invalid Rust).

With cast hint propagation:

```
After:          let __mv0: u8 = (((c.Status | (FlagB as u8)) | 0x20) as u8);
                c = PushByte(c, __mv0);
```

The `castHint = "u8"` flows into the `BinaryExpr` handler, which passes it to sub-expressions. When encountering the constant `FlagB` (type `i32`), it detects the mismatch and wraps it with `as u8`.

### Impact

Eliminated **17 additional `.clone()` calls** (83 to 66), increasing temp bindings from 161 to 189.

---

## Optimization 4: Conditional Multi-Value Return Clone (Rust)

**Pattern**: `return variable, expression`

The baseline rule clones the first result in every multi-value return. This optimization only clones when a later result actually references the same identifier.

### Before

```
Go source:      return c, value

Rust output:    return (c.clone(), value);     // always cloned
```

### After

The emitter checks whether any result at index > 0 references the same identifier as result 0:

```
Go source:      return c, value     // value doesn't reference c

Rust output:    return (c, value);  // no clone needed
```

```
Go source:      return c, c.Memory[0x100 + int(c.SP)]   // later result references c

Rust output:    return (c.clone(), c.Memory[...]);  // clone needed
```

### Implementation

In `PostVisitReturnStmt` (`rust_stmt.go`), when processing the first return result (index 0) of a multi-value return, the emitter walks expressions at index 1..N with `ExprContainsIdent` to check for references to the same variable. If no later result references the first, `.clone()` is not appended. This is an emitter-level optimization (not pass-based) since it determines whether to add `.clone()` in the first place.

---

## Optimization 5: `std::move()` for Multi-Value Returns (C++)

**Pattern**: `return variable, expression` where variable is a struct

### Before

```
Go source:      return c, value

C++ output:     return std::make_tuple(c, value);    // c is copied into tuple
```

### After

When the first result is a struct and later results don't reference it:

```
C++ output:     return std::make_tuple(std::move(c), value);  // c is moved into tuple
```

---

## Optimization 6: `std::mem::take` for Struct Field Reassignment (Rust)

**Flag**: `--optimize-moves`

**Pattern**: `state.Field = Function(state.Field, ...)`

When the left-hand side of an assignment is a struct field (a `SelectorExpr` like `state.C`) and the same field appears as an argument to the called function, the emitter replaces the clone with `std::mem::take(&mut state.Field)`.

### Why `canMoveArg` Doesn't Apply Here

`canMoveArg` handles local variables (`c = func(c)`), where Rust can move the variable directly. Struct fields are different — you cannot move out of a field while the parent struct is still alive. Rust requires the field to remain valid.

`std::mem::take` solves this by replacing the field with its `Default` value (for `Vec<u8>`, that's an empty vec) and returning the old value. The field is always valid, and the caller gets ownership of the old value without cloning.

### Before

```
Go source:      state.C = cpu.Run(state.C, 100000)

Rust output:    state.C = cpu::Run(state.C.clone(), 100000);   // 64KB deep copy
```

### After

```
Rust output:    state.C = cpu::Run(std::mem::take(&mut state.C), 100000);   // zero-cost swap
```

### Conditions

1. **LHS is a SelectorExpr**: e.g., `state.C`, `state.BasicState`.
2. **LHS type is a named struct**: Not a primitive/Copy type.
3. **RHS is a CallExpr**: The RHS is a function call.
4. **An argument matches the LHS**: One of the call arguments has the same expression string as the LHS.
5. **No other argument references the field or sub-fields**: Safety check using `exprContainsSelectorPath`.
6. **Not inside a closure**: `funcLitDepth == 0`.

### Implementation

`analyzeMemTakeOpt` (`rust_opt.go`) runs in `PreVisitAssignStmt` after `analyzeMoveOptExtraction`. It stores `memTakeLhsExpr` and `memTakeArgIdx`. In `PostVisitCallExprArg`, when the arg index matches, the emitter sets `OptMeta{IsReassignedSource: true, ReassignedExpr: "state.C"}` on the argument node (which has `.clone()` appended). The `CloneMovePass` then replaces the entire node content with `std::mem::take(&mut state.C)`.

### Impact

This is critical for the c64-v2 emulator which uses a `State` struct containing `C: CPU`. Without this optimization, every CPU operation in the event handler (`LoadProgram`, `SetPC`, `ClearHalted`, `Run`, `printReady`, `scrollScreenUp`) would clone 64KB. With it, all are zero-cost swaps.

---

## Optimization 7: Reference Optimization for Read-Only Parameters

**Flag**: `--optimize-refs`

**Pattern**: Function parameters that are never mutated, returned, or assigned from can be passed by `&T` instead of by value, eliminating the `.clone()` at call sites.

This optimization is implemented as an IR pass (`RefOptPass` in `ref_opt_pass.go`), separate from the emitter and `CloneMovePass`.

### Analysis Pass

Before emitting code, `AnalyzeReadOnlyParams` (`rust_opt.go`) scans every function in the package:

1. **Collect mutated variables**: Any parameter assigned to (LHS of `=`), or whose fields are assigned, is marked mutable.
2. **Collect returned variables**: Parameters that appear in `return` statements are marked (they need ownership to be returned).
3. **Collect assigned-from variables**: Parameters that are assigned whole to another variable (e.g., `x = param`) need ownership.
4. **Build read-only flags**: A parameter is read-only if it's an eligible type (struct or slice, not primitive) and is NOT mutated, returned, or assigned-from.
5. **Exclude callbacks**: Functions used as values (passed to higher-order functions) are skipped because their signatures must match the expected type.

The result is stored in `refOptReadOnly[funcKey]` as a per-parameter boolean array.

### Emitter Annotations

The emitter annotates IR nodes with `OptMeta` for the `RefOptPass`:

**Function parameters**: Each parameter gets `OptMeta{Kind: OptFuncParam, FuncKey: "pkg.Func", ParamIndex: N, IsReadOnly: true/false, IsMutRef: true/false}`.

**Call arguments**: Each argument gets `OptMeta{Kind: OptCallArg, FuncKey: "pkg.Func", ParamIndex: N}` (merged in `PostVisitCallExprArgs`).

### RefOptPass Transformations

The `RefOptPass` runs after `CloneMovePass` in the IR pipeline:

1. **Phase 1**: Walks the tree to collect all `OptFuncParam` annotations into a `funcParams` map.
2. **Phase 2**: For each `OptCallArg`, looks up the corresponding function's param info. If the param is read-only, strips `.clone()` and adds `&` prefix. If mut-ref, adds `&mut` prefix.

**Function signatures** (param nodes):
- Rust: `mut name: T` → `name: &T` (read-only) or `name: &mut T` (mut-ref)
- C++: `Type name` → `const Type& name` (read-only) or `Type& name` (mut-ref)
- C#: `Type name` → `in Type name` (read-only) or `ref Type name` (mut-ref)

### Before

```
Go source:      code := basic.CompileImmediate(state.BasicState, line)

Rust output:    let mut code = basic::CompileImmediate(state.BasicState.clone(), line.clone());
```

### After

```
// CompileImmediate(state: &BasicState, line: String) -- state is read-only
Rust output:    let mut code = basic::CompileImmediate(&state.BasicState, line.clone());
```

The function signature changes from `state: BasicState` to `state: &BasicState`, and the call site replaces `.clone()` with `&`.

### Before (AssembleLines)

```
Go source:      return assembler.AssembleLines(asmLines)

Rust output:    return assembler::AssembleLines(asmLines.clone());
```

### After

```
// AssembleLines(lines: &Vec<String>) -- lines is read-only
Rust output:    return assembler::AssembleLines(&asmLines);
```

### Impact

Eliminated **162 `.clone()` calls** across the c64-v2 codebase by converting read-only parameters to references.

---

## Optimization 8: Return Temp Extraction for Multi-Value Returns

**Flag**: `--optimize-moves`

**Pattern**: `return variable, expression_referencing_variable`

Optimization 4 detects when the first return result needs cloning because a later result references the same identifier. This optimization goes further: when the conflicting later results produce **Copy-type values**, it extracts them into temporary variables before the `return`, eliminating the clone entirely.

### Motivating Example: `PullByte`

```go
func PullByte(c CPU) (CPU, uint8) {
    c.SP = c.SP + 1
    return c, c.Memory[0x100+int(c.SP)]
}
```

The second result `c.Memory[...]` references `c`, so Optimization 4 adds `.clone()` to the first result. But `c.Memory[...]` evaluates to `uint8`, which is a Copy type. We can extract it before the return.

### Before (Optimization 4 only)

```rust
pub fn PullByte(mut c: CPU) -> (CPU, u8) {
    c.SP = (c.SP + 1);
    return (c.clone(), c.Memory[(0x100 + (c.SP as i32)) as usize]);
    //      ^^^^^^^^^ 64KB deep copy
}
```

### After

```rust
pub fn PullByte(mut c: CPU) -> (CPU, u8) {
    c.SP = (c.SP + 1);
    let __mv0: u8 = c.Memory[(0x100 + (c.SP as i32)) as usize];
    return (c, __mv0);
    //      ^ zero-cost move
}
```

### Conditions for Extraction

1. **Multi-value return**: The function returns more than one value.
2. **First result is an identifier**: e.g., `c`.
3. **A later result references the identifier**: `exprContainsIdent` returns true.
4. **The later result is a Copy type**: `isCopyType` returns true (bool, integers, floats).
5. **The expression is stringifiable**: `exprToRustCodeOpt` can generate valid Rust.

### Implementation

In `PreVisitReturnStmt` (`rust_stmt.go`), before emitting `return`, the optimization:

1. Walks each later result (index 1..N) that references the first result's identifier.
2. For each that is Copy-type and stringifiable via `exprToRustCodeOpt`, builds `let __mv{N}: {type} = {expr};` as IR nodes and stores them in `returnTempPreamble`.
3. Records the replacement in `returnTempReplacements[index] = tempName`.

In `PostVisitReturnStmt`:
- Emits the `returnTempPreamble` IR nodes before the return statement.
- For replaced results: substitutes the temp variable name.
- For index 0: if all conflicting results were extracted, skips `.clone()`.

### Impact

This is critical for functions like `PullByte` which is called inside the `Step` loop (up to 100,000 cycles in `cpu.Run`). Each PullByte call previously deep-copied 64KB. With this optimization, it's a zero-cost move. For the CLR command alone, this eliminates hundreds of megabytes of unnecessary allocations.

---

## Optimization 9: `len()` Argument Clone Elimination

**Flag**: `--optimize-moves`

**Pattern**: `len(collection)` where collection is a Vec/slice or String

The `len()` built-in is read-only — it only needs a reference to compute the length. The emitter already prepends `&` to `len()` arguments. However, `PostVisitCallExprArg` was still adding `.clone()` because it saw a non-Copy type (slice or string) passed to a non-`append` function.

### Before

```
Go source:      if i >= len(lines) { break }

Rust output:    if (i >= len(&lines.clone())) { break; }
                //              ^^^^^^^^ deep copies entire Vec<String> just for length!
```

Inside a loop with ~1000 assembly lines, this clones the entire string vector **twice per iteration** (the length check appears in two places in `AssembleLines`).

### After

```
Rust output:    if (i >= len(&lines)) { break; }
```

### Implementation

The `len()` builtin is handled in `PostVisitCallExprArgs` (`rust_expr.go`). Its argument is preserved as an IR tree node (not flattened to string), so `OptMeta` annotations survive for `CloneMovePass` and `RefOptPass` processing. The emitter wraps the argument with `&` (for slice len) or calls `.len()` (for string len). The `CloneMovePass` recognizes `len` as a builtin callee via `isBuiltinCallee()` and removes the redundant `.clone()`.

### Impact

This is a systemic optimization. The `len(&x.clone())` pattern appeared pervasively in every loop condition throughout the tokenizer, parser, and assembler. For the CLR command compiling ~1000 assembly lines, this eliminated ~2000 unnecessary vector clones per command invocation.

---

## Optimization 10: Move Semantics for Slice/Vec Arguments

**Flag**: `--optimize-moves`

**Pattern**: `variable = Function(variable, ...)` where variable is a slice/Vec type

`canMoveArg` previously only allowed moves for struct types. Slice types (`[]T`) always got `.clone()` unless the call was `append`. This meant patterns like `allBytes = AppendLineBytes(allBytes, lineBytes)` would deep-copy the growing byte buffer on every iteration.

### Before

```
Go source:      allBytes = AppendLineBytes(allBytes, lineBytes)

Rust output:    allBytes = AppendLineBytes(allBytes.clone(), &lineBytes);
                //                         ^^^^^^^^^^^^^^^^ deep copies growing buffer!
```

In `AssembleLines`, this runs ~1000 times. The buffer grows each iteration, so the total allocation is O(n²).

### After

```
Rust output:    allBytes = AppendLineBytes(allBytes, &lineBytes);
                //                         ^^^^^^^^ zero-cost move
```

### Implementation

The `canMoveArg` check was added to the slice-type clone path in `PostVisitCallExprArg`, alongside the existing `currentCallIsAppend` check. The same check was also added for named slice type aliases (e.g., `type AST []Statement`).

The conditions are the same as for structs:
1. The argument is an identifier on the LHS of the assignment.
2. The identifier appears only once in the call arguments.
3. Not inside a closure.

### Impact

Eliminated O(n²) allocation behavior in the assembler. For the CLR command with ~1000 lines, this removed ~1000 buffer clones.

---

## Optimization 11: Double-Clone Suppression for Vec Element Access

**Flag**: `--optimize-moves`

**Pattern**: `Function(collection[index])` where element type is non-Copy (string or struct)

When a non-Copy element is accessed from a Vec and passed to a function, two `.clone()` calls were emitted:

1. `PostVisitIndexExpr` adds `.clone()` because Rust doesn't allow moving out of indexed collections.
2. `PostVisitCallExprArg` adds another `.clone()` because it sees a non-Copy type argument.

The first clone already produces an owned value. The second is redundant.

### Before

```
Go source:      lineBytes := StringToBytes(lines[i])

Rust output:    let mut lineBytes = StringToBytes(lines[i as usize].clone().clone());
                //                                                  ^^^^^^^^^^^^^^^^ two clones!
```

### After

```
Rust output:    let mut lineBytes = StringToBytes(lines[i as usize].clone());
                //                                                  ^^^^^^^^ one clone (necessary)
```

### Implementation

An `argAlreadyCloned` flag is set in `PostVisitIndexExpr` (`rust_expr.go`) when a `.clone()` is emitted for a non-Copy vec element while inside a call expression argument (`callExprArgDepth > 0`). In `PostVisitCallExprArg`, if this flag is set, the argument gets `OptMeta{IsElementCopy: true}` instead of having another `.clone()` appended. The `CloneMovePass` sees `IsElementCopy` and leaves the node unchanged (no clone to add, no clone to remove).

The flag is reset at the start of each argument in `PreVisitCallExprArg`.

### Impact

Eliminated ~1000 redundant string clones per CLR command in the assembler path (one per assembly line).

---

## Summary of Results

### Clone Reduction (Rust, c64-v2 emulator)

| Phase | Optimizations | Clones Removed |
|-------|--------------|----------------|
| Optimization 1 | Move detection for reassigned variables | -192 |
| Optimization 2 | Temporary variable extraction for field accesses | -44 |
| Optimization 3 | Type conversion support in expression extraction | -17 |
| Optimization 4 | Conditional multi-value return clone | included above |
| Optimization 6 | `std::mem::take` for struct field reassignment | included in move count |
| Optimization 7 | Reference optimization for read-only parameters | -162 |
| Optimization 8 | Return temp extraction for multi-value returns | included in move count |
| Optimizations 9-11 | `len()` clone elimination, slice move, double-clone suppression | included in move count |
| **Total** | **`--optimize-moves --optimize-refs`** | **322 move + 162 ref = 484 clones eliminated** |

### Implementation Files

- `compiler/rust_expr.go` -- Rust expression emission, `PostVisitCallExprArg` (clone/move annotation), `PostVisitCallExprArgs` (OptMeta merging), `PostVisitIndexExpr` (element clone)
- `compiler/rust_stmt.go` -- Rust statement emission, `PreVisitAssignStmt` (move analysis triggers), `PostVisitReturnStmt` (return clone/move), return temp extraction
- `compiler/rust_opt.go` -- `RustOptState`: `canMoveArg`, `analyzeMoveOptExtraction`, `analyzeMemTakeOpt`, `exprToRustCodeOpt`, `moveOptInsideExtractedCall`, read-only analysis accumulation
- `compiler/clone_move_pass.go` -- `CloneMovePass` IR pass: clone removal (Rust), `std::move()` wrapping (C++), `std::mem::take`, temp extraction assembly
- `compiler/ref_opt_pass.go` -- `RefOptPass` IR pass: param signature and call-site reference transformations (Rust, C++, C#)
- `compiler/irnode.go` -- `IRNode`, `OptMeta`, `OptKind`, `TempExtraction`, tree helpers
- `compiler/pass_manager.go` -- Pass orchestration: frontend passes → backend pipeline (codegen + IR passes)
- `compiler/cpp_emitter.go` -- C++ emitter with optimizations (1, 5)

### Flags

| Flag | Optimizations | Description |
|------|--------------|-------------|
| `--optimize-moves` | 1, 2, 3, 4, 6, 8, 9, 10, 11 | Move semantics, temp extraction, `std::mem::take`, `len()` clone elimination, slice moves, double-clone suppression |
| `--optimize-refs` | 7 | Reference optimization for read-only parameters |

---

## Future Work: Eliminating Closure Clones

The largest remaining category of clones (28 in the CPU emulator) occurs inside closures. Rust's `FnMut` closures capture variables by `&mut T`, which prevents moving the value out. The emitter must add `.clone()` to preserve correctness, even when the variable is immediately reassigned:

```rust
// Inside RunLoop closure -- state captured by &mut
state = handleKeyInput(state.clone(), key);  // clone required by FnMut
```

The programmer knows the value comes back (it's reassigned on the same line), but Rust's borrow checker doesn't analyze across the function call boundary. It enforces a simple structural rule: you cannot move out of `&mut T`, period.

Two complementary approaches can eliminate these clones.

### Approach A: `RunLoop` API Redesign (Recommended First)

**Problem**: `RunLoop` takes a closure that captures mutable state. The closure is `FnMut` because it's called every frame, and it mutates the captured variables.

**Solution**: Thread state through the function signature instead of capturing it:

```go
// Current API -- state captured by closure
graphics.RunLoop(w, func(w graphics.Window) bool {
    state = handleKeyInput(state, key)
    // ... render using state ...
    return true
})

// Proposed API -- state passed as parameter, returned each frame
graphics.RunLoop(w, state, func(w graphics.Window, state AppState) (AppState, bool) {
    state = handleKeyInput(state, key)
    // ... render using state ...
    return state, true
})
```

In the proposed API, `RunLoop` owns the state and passes it **by value** into each frame call, then takes back the returned value for the next iteration. The closure receives ownership each call, so Rust allows moves freely. No captures, no clones.

**Generated Rust**:

```rust
// Current: clone required
graphics::RunLoop(w.clone(), |w: graphics::Window| -> bool {
    state = handleKeyInput(state.clone(), key);  // 64KB deep copy
    true
});

// Proposed: zero-cost move
graphics::RunLoop(w.clone(), state, |w: graphics::Window, state: AppState| -> (AppState, bool) {
    state = handleKeyInput(state, key);  // zero-cost move
    (state, true)
});
```

**Impact on backends**:

- **Rust**: Eliminates all closure clones. The state is moved in and out each frame at zero cost.
- **C++**: `std::move()` applied to the state parameter. Same zero-cost benefit.
- **JS**: Works naturally. JS has no ownership issues, so passing state through is just passing a reference. The `requestAnimationFrame` callback passes `newState` to the next iteration.
- **C#**: Value types passed through, no issue.

**Scope**: Changes the `RunLoop` runtime API, all backend emitters' `RunLoop` code generation, and all example programs that use `RunLoop`. Should be done on a separate branch.

### Approach B: `Option<T>` Wrapping in Rust Emitter

**Problem**: For closures where the API cannot be changed (third-party libraries, patterns beyond `RunLoop`), captured struct variables still require `.clone()`.

**Solution**: Wrap closure-captured struct variables in `Option<T>` and use `.take()` to move the value out temporarily:

```rust
// Instead of:
let mut state: AppState = ...;
graphics::RunLoop(w.clone(), |w: graphics::Window| -> bool {
    state = handleKeyInput(state.clone(), key);  // 64KB deep copy
    true
});

// Generate:
let mut state: Option<AppState> = Some(...);
graphics::RunLoop(w.clone(), |w: graphics::Window| -> bool {
    let __tmp = state.take().unwrap();           // move out (zero cost)
    state = Some(handleKeyInput(__tmp, key));     // move back in (zero cost)
    true
});
```

**How `Option<T>` works under the hood**:

`Option<T>` is an enum: `Some(value)` or `None`. Stored inline -- no heap allocation. For a struct, it's the struct itself plus a 1-byte tag.

`.take()` performs:
1. Reads the tag (it's `Some`)
2. Moves the inner value out (zero-cost: copies pointer/length/capacity, not the 64KB data)
3. Writes `None` in place (sets tag byte to "empty")
4. Returns the moved-out value

The sequence `.take()` -> call function -> `Some(result)` costs two tag-byte writes plus two shallow struct moves. Compared to `.clone()` which deep-copies the entire `Memory` array, this is effectively free.

**Why `Option` is allowed but direct move is not**:

Rust's `FnMut` captures by `&mut T`. You cannot move out of `&mut T` because the closure may be called again and the variable would be empty. `Option<T>` provides a valid "empty" state (`None`), so the variable is never in an undefined state -- even temporarily between `.take()` and `Some(...)`. Rust allows `.take()` on `&mut Option<T>` because `Option` is always valid.

**Emitter changes required**:

1. **Detection**: Identify struct variables captured by closures that follow the `var = func(var, ...)` reassignment pattern (same pattern `canMoveArg` detects, currently blocked by `funcLitDepth > 0`).
2. **Declaration wrapping**: Emit `let mut var: Option<Type> = Some(...)` instead of `let mut var: Type = ...`.
3. **Call site**: Emit `var = Some(func(var.take().unwrap(), ...))` instead of `var = func(var.clone(), ...)`.
4. **Read sites**: Every other read of `var` inside the closure needs unwrapping. Field access `state.C` becomes `state.as_ref().unwrap().C`. This is the trickiest part.

**Risk**: Point 4 is error-prone. Missing an unwrap site produces a Rust compile error (not a silent bug), so failures are safe but could require iteration.

**Flag**: `--optimize-closure-moves` (default off), independent of `--optimize-moves`.

### Relationship Between Approaches

The two approaches are **orthogonal**:

- **Approach A** solves the problem at the API design level. State flows through function signatures instead of being captured. It's the "do it right" approach and should come first.
- **Approach B** is a Rust-specific workaround for closures that genuinely need to capture mutable structs (cases where the API can't be changed). It's useful beyond `RunLoop` for any `FnMut` closure pattern.

Approach A eliminates the need for Approach B in the `RunLoop` case, but Approach B remains valuable for other closure patterns.
