package main

// This file demonstrates ALL code patterns that are NOT allowed by goany.
// Each function shows a specific disallowed pattern.
// Running goany on this file will produce semantic errors.

// WARNING: Constants without explicit type
const MyConst = 42 // should be: const MyConst int = 42

// ERROR: Address-of operator is not supported
func addressOfError() {
	x := 5
	p := &x // error: taking address not allowed
	_ = p
}

// ERROR: Pointer types are not supported
func pointerTypeError() {
	var p *int // error: pointer type not allowed
	_ = p
}

// ERROR: Defer statements are not supported
func deferError() {
	defer println("cleanup") // error: defer not allowed
}

// ERROR: Goroutines are not supported
func goroutineError() {
	go func() {}() // error: go keyword not allowed
}

// ERROR: Channel types are not supported
func channelError() {
	ch := make(chan int) // error: channel type not allowed
	_ = ch
}

// NOTE: Maps ARE supported with limitations (see MAP LIMITATIONS section below)

// ERROR: Select statements are not supported
func selectError() {
	ch := make(chan int)
	select { // error: select not allowed
	case <-ch:
	}
}

// ERROR: Labeled statements are not supported
func labelError() {
label: // error: label not allowed
	_ = 0
	goto label // error: goto not allowed
}

// ERROR: Method receivers are not supported
type MyType struct {
	Value int
}

func (m MyType) GetValue() int { // error: method receiver not allowed
	return m.Value
}

// ERROR: Init functions are not supported
func init() { // error: init() not allowed
}

// ERROR: Variadic functions are not supported
func variadicError(args ...int) { // error: variadic not allowed
	_ = args
}

// ERROR: Named return values are not supported
func namedReturnError() (result int) { // error: named return not allowed
	return 0
}

// ERROR: Non-empty interfaces are not supported
type Reader interface {
	Read() int // error: interface methods not allowed
}

// ERROR: Struct embedding is not supported
type Base struct {
	X int
}

type Derived struct {
	Base // error: embedding not allowed
	Y int
}

// ERROR: Type switch is not supported
func typeSwitchError() {
	var x interface{}
	switch x.(type) { // error: type switch not allowed
	case int:
	}
}

// ERROR: Range over inline slice literal is not supported
func rangeInlineError() {
	for _, x := range []int{1, 2, 3} { // error: inline slice literal not allowed
		_ = x
	}
}

// ERROR: Nil comparison is not supported
func nilCompareError() {
	var s []int
	if s == nil { // error: nil comparison not allowed
	}
}

// ERROR: Iota is not supported
const (
	A = iota // error: iota not allowed
	B
	C
)


// ERROR: Collection mutation during iteration
func mutationDuringIterationError() {
	items := []int{1, 2, 3}
	for i := 0; i < len(items); i++ {
		items = append(items, 4) // error: mutation during iteration
	}
}

// ============================================
// RUST-SPECIFIC OWNERSHIP PATTERNS
// ============================================

// --- String Concatenation Patterns (Rust move semantics) ---

// ERROR: String variable reuse after concatenation
// In Rust, strings are moved (not copied) when concatenated.
// After `a + b`, the variable `a` is consumed and cannot be used again.
func stringReuseError() {
	a := "hello"
	b := " world"
	c := a + b // a is moved/consumed here
	d := a + c // error: a was already moved in previous line
	_ = d
}

// ERROR: Self-referencing string concatenation
// Pattern: s += s + "..." is invalid because s is both borrowed (for +=)
// and moved (in the + expression) simultaneously.
func selfRefStringError() {
	s := "hello"
	s += s + " world" // error: s borrowed and moved in same statement
}

// --- Slice/Array Ownership Patterns ---

// ERROR: Same variable used multiple times in expression (Rust move semantics)
func process(a []int, b []int) []int {
	return append(a, b...)
}

func sameVarMultipleTimesError() {
	data := []int{1, 2, 3}
	result := process(data, data) // error: data used twice, moved twice
	_ = result
}

// ERROR: Nested function calls share non-Copy variable (Rust move semantics)
func outer(x []int) []int {
	return x
}

func inner(x []int) []int {
	return x
}

func nestedCallsError() {
	data := []int{1, 2, 3}
	result := outer(inner(data)) // error: data passed to both inner and outer
	_ = result
}

// ERROR: Multiple closures capture same variable (Rust borrow checker)
func multipleClosuresError() {
	data := []int{1, 2, 3}

	fn1 := func() {
		_ = data // first closure captures data
	}

	fn2 := func() {
		_ = data // error: second closure also captures data
	}

	fn1()
	fn2()
}

// ERROR: Mutation of variable after closure capture
func mutationAfterCaptureError() {
	data := []int{1, 2, 3}

	fn := func() {
		_ = data // closure captures data
	}

	data = append(data, 4) // error: mutating after capture
	fn()
}

// ERROR: Slice self-assignment (Rust borrow checker)
func sliceSelfAssignError() {
	items := []int{1, 2, 3}
	items = append(items, items[0]) // error: items borrowed and mutated simultaneously
}

// ============================================
// C#-SPECIFIC PATTERNS
// ============================================

// ERROR: Variable shadowing (C# does not allow shadowing within same function)
// In C#, you cannot declare a variable with the same name in a nested scope
// if a variable with that name exists in an outer scope of the same function.
func variableShadowingError() {
	col := 0 // outer scope variable
	for {
		if col >= 10 {
			break
		}
		col := 1 // error: shadows outer 'col' - C# doesn't allow this
		_ = col
	}
	_ = col
}

// ERROR: Variable shadowing in if block
func variableShadowingIfError() {
	x := 5 // outer scope
	if true {
		x := 10 // error: shadows outer 'x'
		_ = x
	}
	_ = x
}

// ============================================
// MAP LIMITATIONS
// ============================================
// Maps ARE supported but with the following limitations.

// ERROR: Nested maps are not supported
// Map values cannot themselves be maps.
func nestedMapError() {
	m := make(map[string]map[string]int) // error: nested maps not allowed
	_ = m
}

// ERROR: Map literals are not supported
// Inline map initialization is not allowed.
func mapLiteralError() {
	m := map[string]int{"a": 1, "b": 2} // error: map literals not allowed
	_ = m
}

// ERROR: Range over maps is not supported
// Map iteration order is undefined and varies across languages.
func rangeMapError() {
	m := make(map[string]int)
	m["a"] = 1
	for k, v := range m { // error: range over maps not allowed
		_ = k
		_ = v
	}
}

// ERROR: Structs as map keys are not supported
// Only primitive types (string, int, bool, etc.) can be map keys.
type Point struct {
	X, Y int
}

func structKeyError() {
	m := make(map[Point]string) // error: struct key type not allowed
	_ = m
}

func main() {
}
