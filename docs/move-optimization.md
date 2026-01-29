# Move Optimization for Struct Arguments in Rust and C++ Emitters

## Problem Statement

The ULang transpiler generates Rust and C++ code from Go sources. Go's value semantics mean structs are implicitly copied when passed to functions. In Go, this is handled by the runtime with cheap stack copies. However, when transpiled:

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

## Optimization 1: Move Detection for Reassigned Variables

**Pattern**: `variable = Function(variable, ...)`

When a struct variable appears on both the left-hand side and as a function argument, the old value is being consumed and replaced. This is semantically a **move**, not a copy.

### Conditions for Safe Move

All conditions must be met:

1. **Assignment context**: The variable appears on the LHS of the current assignment statement.
2. **Single reference**: The variable appears only once across all arguments of the call (count <= 1).
3. **Not inside a closure**: The code is not inside a function literal (`funcLitDepth == 0`). Rust's `FnMut` closures cannot move captured variables.

### Rust Implementation

When conditions are met, the emitter **omits `.clone()`**, allowing Rust's move semantics to transfer ownership:

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

When conditions are met, the emitter wraps the argument with `std::move()`:

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
3. **Convertible to string**: The `exprToString` function can generate a valid Rust expression for the argument.
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

### Token Buffer Replacement

The extraction uses a marker-based approach in the GIR (Generic Intermediate Representation) token buffer:

1. `PreVisitCallExprArg` records the token buffer position before the argument is emitted.
2. `PostVisitCallExprArg` truncates the buffer back to the marker and emits the temp variable name instead.
3. A depth guard (`len(currentCallArgIdentsStack) == 1`) ensures nested `CallExpr` nodes (such as type conversions) don't interfere with the replacement.

### Impact

Eliminated **44 additional `.clone()` calls** (127 to 83), generating 161 temporary variable bindings.

---

## Optimization 3: Type Conversion Support in Expression Extraction

**Pattern**: `variable = Function(variable, variable.Field[TypeConversion(expr)])`

Go type conversions like `int(addr)` and `uint8(c.A & val)` are represented as `CallExpr` nodes in the Go AST. The initial `exprToString` function could not handle `CallExpr`, causing extraction to fail for expressions containing type casts.

### Before

Expressions with type conversions returned `""` from `exprToString`, blocking extraction:

```
Go source:      c = doADC(c, c.Memory[int(addr)])

Could not extract c.Memory[int(addr)] because int(addr) is a CallExpr.
Result:         c = doADC(c.clone(), c.Memory[(addr as i32) as usize]);
```

### After

`exprToString` now detects single-argument `CallExpr` where the function is a Go built-in type name (found in `rustTypesMap`), and emits a Rust `as` cast:

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

`PostVisitReturnStmtResult` saves the current `ReturnStmt` node in `PreVisitReturnStmt`. When processing index 0, it walks the expressions at index 1..N with `exprContainsIdent` to check for references to the same variable.

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

## Summary of Results

### Clone Reduction for CPU Struct (Rust)

| Phase | `c.clone()` Count | Temp Bindings | Change |
|-------|-------------------|---------------|--------|
| Baseline (before optimization) | ~319 | 0 | -- |
| Optimization 1 (move detection) | 127 | 0 | -192 |
| Optimization 2 (field extraction) | 83 | 161 | -44 |
| Optimization 3 (type conversion) | 66 | 189 | -17 |
| **Total reduction** | | | **-253 (79%)** |

### Remaining Clones (Genuinely Required)

| Category | Count | Reason |
|----------|-------|--------|
| Inside closures | 28 | Rust FnMut cannot move captured variables |
| Condition expressions | 20 | Variable not reassigned, used later |
| Field assignment LHS | 8 | `c.A = ReadIndirect(c, zp)` -- c needed alive for field assignment |
| Nested function calls consuming c | 6 | Both outer and inner call consume c |
| Different LHS variable | 2 | `let addr = GetIndirect(c, zp)` -- c not reassigned |
| Multi-value return with shared ref | 1 | Both results reference c |
| Other | 1 | -- |

### Implementation Files

- `compiler/rust_emitter.go` -- Rust emitter with all optimizations (1-4)
- `compiler/cpp_emitter.go` -- C++ emitter with optimizations (1, 5)

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
