package compiler

import (
	"bytes"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// runCanonicalize parses Go source, type-checks it, runs the CanonicalizePass
// with the given backends, and returns the transformed source as a string.
func runCanonicalize(t *testing.T, src string, backends BackendSet) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	conf := types.Config{
		Importer: importer.Default(),
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}
	typePkg, err := conf.Check("test", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("type check error: %v", err)
	}

	pkg := &packages.Package{
		Name:      typePkg.Name(),
		Fset:      fset,
		Syntax:    []*ast.File{file},
		TypesInfo: info,
		Types:     typePkg,
	}

	pass := &CanonicalizePass{Backends: backends}
	visitors := pass.Visitors(pkg)
	for _, visitor := range visitors {
		pass.PreVisit(visitor)
	}
	for _, visitor := range visitors {
		for _, f := range pkg.Syntax {
			ast.Walk(visitor, f)
		}
	}
	visited := make(map[string]struct{})
	for _, visitor := range visitors {
		pass.PostVisit(visitor, visited)
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		t.Fatalf("printer error: %v", err)
	}
	return buf.String()
}

// assertContains checks that the result contains the expected substring.
func assertContains(t *testing.T, result, expected string) {
	t.Helper()
	if !strings.Contains(result, expected) {
		t.Errorf("expected result to contain %q, got:\n%s", expected, result)
	}
}

// assertNotContains checks that the result does NOT contain the substring.
func assertNotContains(t *testing.T, result, unexpected string) {
	t.Helper()
	if strings.Contains(result, unexpected) {
		t.Errorf("expected result to NOT contain %q, got:\n%s", unexpected, result)
	}
}

// assertValidGo verifies that the result is valid Go source.
func assertValidGo(t *testing.T, result string) {
	t.Helper()
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "test_output.go", result, parser.AllErrors)
	if err != nil {
		t.Errorf("transformed source is not valid Go: %v\nsource:\n%s", err, result)
	}
}

// =============================================
// MultiAssignSplit tests
// =============================================

func TestCanonicalizeMultiAssign_Define(t *testing.T) {
	src := `package test

func main() {
	a, b := 1, 2
	_ = a
	_ = b
}
`
	// Reference: expected code after transform (C++ needs single-var declarations)
	//   a, b := 1, 2
	// → a := 1
	//   b := 2
	want := `func main() {
	a := 1
	b := 2
	_ = a
	_ = b
}`
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeMultiAssign_Assign(t *testing.T) {
	src := `package test

func main() {
	var a, b int
	a, b = 1, 2
	_ = a
	_ = b
}
`
	// Reference: expected code after transform
	//   a, b = 1, 2
	// → a = 1
	//   b = 2
	wantA := "a = 1"
	wantB := "b = 2"
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, wantA)
	assertContains(t, result, wantB)
	assertNotContains(t, result, "a, b =")
}

func TestCanonicalizeMultiAssign_CommaOkMap(t *testing.T) {
	src := `package test

func main() {
	m := make(map[string]int)
	v, ok := m["key"]
	_ = v
	_ = ok
}
`
	// Reference: no transform (comma-ok map access is NOT split)
	//   v, ok := m["key"] → v, ok := m["key"]  (unchanged)
	want := `v, ok := m["key"]`
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeMultiAssign_TypeAssertion(t *testing.T) {
	src := `package test

func main() {
	var x interface{} = 42
	v, ok := x.(int)
	_ = v
	_ = ok
}
`
	// Reference: no transform (comma-ok type assertion is NOT split)
	//   v, ok := x.(int) → v, ok := x.(int)  (unchanged)
	want := "v, ok := x.(int)"
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeMultiAssign_MultiReturn(t *testing.T) {
	src := `package test

func pair() (int, int) { return 1, 2 }

func main() {
	a, b := pair()
	_ = a
	_ = b
}
`
	// Reference: no transform (multi-return function call is NOT split)
	//   a, b := pair() → a, b := pair()  (unchanged)
	want := "a, b := pair()"
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeMultiAssign_Multiple(t *testing.T) {
	src := `package test

func main() {
	a, b := 1, 2
	c, d := 3, 4
	_ = a
	_ = b
	_ = c
	_ = d
}
`
	// Reference: expected code after transform (both multi-assigns split)
	//   a, b := 1, 2  →  a := 1; b := 2
	//   c, d := 3, 4  →  c := 3; d := 4
	want := `func main() {
	a := 1
	b := 2
	c := 3
	d := 4
	_ = a
	_ = b
	_ = c
	_ = d
}`
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeMultiAssign_SkipWhenNoCpp(t *testing.T) {
	src := `package test

func main() {
	a, b := 1, 2
	_ = a
	_ = b
}
`
	// Reference: no transform (Rust supports tuples, only C++ needs split)
	//   a, b := 1, 2 → a, b := 1, 2  (unchanged)
	want := "a, b := 1, 2"
	result := runCanonicalize(t, src, BackendRust)
	assertContains(t, result, want)
}

// =============================================
// ShadowingRename tests
// =============================================

func TestCanonicalizeShadowing_Basic(t *testing.T) {
	src := `package test

func main() {
	x := 1
	if true {
		x := 2
		_ = x
	}
	_ = x
}
`
	// Reference: expected code after transform (C# doesn't allow shadowing)
	//   inner x := 2  → x_2 := 2
	//   inner _ = x   → _ = x_2
	//   outer x := 1  → x := 1  (unchanged)
	want := `func main() {
	x := 1
	if true {
		x_2 := 2
		_ = x_2
	}
	_ = x
}`
	result := runCanonicalize(t, src, BackendCSharp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeShadowing_TripleNesting(t *testing.T) {
	src := `package test

func main() {
	x := 1
	if true {
		x := 2
		if true {
			x := 3
			_ = x
		}
		_ = x
	}
	_ = x
}
`
	// Reference: expected code after transform (each shadow level gets a suffix)
	//   outer x := 1  → x := 1   (unchanged)
	//   inner x := 2  → x_2 := 2
	//   deepest x := 3 → x_3 := 3
	want := `func main() {
	x := 1
	if true {
		x_2 := 2
		if true {
			x_3 := 3
			_ = x_3
		}
		_ = x_2
	}
	_ = x
}`
	result := runCanonicalize(t, src, BackendCSharp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeShadowing_ForLoop(t *testing.T) {
	src := `package test

func main() {
	i := 0
	for i := 0; i < 10; i++ {
		_ = i
	}
	_ = i
}
`
	// Reference: expected code after transform
	//   for i := 0; i < 10; i++  → for i_2 := 0; i_2 < 10; i_2++
	wantLoop := "for i_2 := 0; i_2 < 10; i_2++"
	wantBody := "_ = i_2"
	result := runCanonicalize(t, src, BackendCSharp)
	assertValidGo(t, result)
	assertContains(t, result, wantLoop)
	assertContains(t, result, wantBody)
}

func TestCanonicalizeShadowing_NoConflict(t *testing.T) {
	src := `package test

func main() {
	x := 1
	y := 2
	_ = x
	_ = y
}
`
	// Reference: no transform (no shadowing, different variable names)
	want := `func main() {
	x := 1
	y := 2
	_ = x
	_ = y
}`
	result := runCanonicalize(t, src, BackendCSharp)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeShadowing_BlankIdent(t *testing.T) {
	src := `package test

func main() {
	_ = 1
	if true {
		_ = 2
	}
}
`
	// Reference: no transform (blank identifiers are never renamed)
	//   _ = 1; _ = 2 → _ = 1; _ = 2  (unchanged, no __2)
	result := runCanonicalize(t, src, BackendCSharp)
	assertValidGo(t, result)
	assertNotContains(t, result, "__2")
}

func TestCanonicalizeShadowing_SkipWhenNoCSharp(t *testing.T) {
	src := `package test

func main() {
	x := 1
	if true {
		x := 2
		_ = x
	}
	_ = x
}
`
	// Reference: no transform (Rust allows shadowing, only C#/C++/Java need rename)
	//   inner x := 2 → x := 2  (unchanged, no x_2)
	result := runCanonicalize(t, src, BackendRust)
	assertNotContains(t, result, "x_2")
}

// =============================================
// FieldNameConflict tests
// =============================================

func TestCanonicalizeFieldConflict_Basic(t *testing.T) {
	src := `package test

type Baz int

type Foo struct {
	Baz Baz
}

func main() {
	f := Foo{Baz: 1}
	_ = f.Baz
}
`
	// Reference: expected code after transform (C++ can't have field name = type name)
	//   struct field:  Baz Baz     → BazVal Baz
	//   composite lit: Baz: 1      → BazVal: 1
	//   field access:  f.Baz       → f.BazVal
	wantStruct := "BazVal Baz"
	wantLit := "BazVal: 1"
	wantAccess := "f.BazVal"
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, wantStruct)
	assertContains(t, result, wantLit)
	assertContains(t, result, wantAccess)
}

func TestCanonicalizeFieldConflict_NoConflict(t *testing.T) {
	src := `package test

type Bar int

type Foo struct {
	X Bar
}

func main() {
	f := Foo{X: 1}
	_ = f.X
}
`
	// Reference: no transform (field name "X" doesn't match type name "Bar")
	//   X Bar → X Bar  (unchanged, no XVal)
	wantStruct := "X Bar"
	result := runCanonicalize(t, src, BackendCpp)
	assertValidGo(t, result)
	assertContains(t, result, wantStruct)
	assertNotContains(t, result, "XVal")
}

func TestCanonicalizeFieldConflict_SkipWhenNoCpp(t *testing.T) {
	src := `package test

type Baz int

type Foo struct {
	Baz Baz
}

func main() {
	f := Foo{Baz: 1}
	_ = f.Baz
}
`
	// Reference: no transform (Rust allows field name = type name, only C++ needs rename)
	//   Baz Baz → Baz Baz  (unchanged, no BazVal)
	result := runCanonicalize(t, src, BackendRust)
	assertNotContains(t, result, "BazVal")
}

// =============================================
// Integration: multiple transforms together
// =============================================

func TestCanonicalizeAllTransforms(t *testing.T) {
	src := `package test

type Baz int

type Foo struct {
	Baz Baz
}

func main() {
	a, b := 1, 2
	x := 10
	if true {
		x := 20
		_ = x
	}
	f := Foo{Baz: 3}
	_ = a
	_ = b
	_ = x
	_ = f.Baz
}
`
	// Reference: all backend-conditional transforms applied together
	//   Multi-assign split (C++):  a, b := 1, 2 → a := 1; b := 2
	//   Shadowing rename (C#):     inner x := 20 → x_2 := 20
	//   Field conflict (C++):      Baz Baz → BazVal Baz
	wantMultiAssign := "a := 1"
	wantShadow := "x_2 := 20"
	wantField := "BazVal Baz"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantMultiAssign)
	assertContains(t, result, "b := 2")
	assertContains(t, result, wantShadow)
	assertContains(t, result, wantField)
}

// =============================================
// Rust Ownership Transform tests
// =============================================

// --- 1. Self-referencing += concatenation ---

func TestCanonicalizeRust_SelfRefConcat(t *testing.T) {
	src := `package test

func main() {
	x := "hello"
	x += x + " world"
	_ = x
}
`
	// Reference: expected code after transform (Rust can't borrow x mutably and immutably)
	//   x += x + " world"
	// → _t0 := x + " world"
	//   x += _t0
	wantTemp := `_t0 := x + " world"`
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
	assertNotContains(t, result, "x += x")
}

func TestCanonicalizeRust_SelfRefConcat_SkipWhenNoRust(t *testing.T) {
	src := `package test

func main() {
	x := "hello"
	x += x + " world"
	_ = x
}
`
	// Reference: no transform (C++ doesn't need self-ref extraction)
	//   x += x + " world" → x += x + " world"  (unchanged)
	want := `x += x + " world"`
	result := runCanonicalize(t, src, BackendCpp)
	assertContains(t, result, want)
}

// --- 2. Slice self-reference ---

func TestCanonicalizeRust_SliceSelfRef_IndexAssign(t *testing.T) {
	src := `package test

func main() {
	s := []string{"a", "b", "c"}
	s[0] = s[1]
	_ = s
}
`
	// Reference: expected code after transform (Rust can't borrow s mutably and immutably)
	//   s[0] = s[1]
	// → _t0 := s[1]
	//   s[0] = _t0
	wantTemp := "_t0 := s[1]"
	wantAssign := "s[0] = _t0"
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
	assertContains(t, result, wantAssign)
}

func TestCanonicalizeRust_SliceSelfRef_Append(t *testing.T) {
	src := `package test

func main() {
	s := []string{"a", "b"}
	s = append(s, s[0])
	_ = s
}
`
	// Reference: expected code after transform (Rust can't borrow s for append and index)
	//   s = append(s, s[0])
	// → _t0 := s[0]
	//   s = append(s, _t0)
	wantTemp := "_t0 := s[0]"
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
}

func TestCanonicalizeRust_SliceSelfRef_CopyTypeSkipped(t *testing.T) {
	src := `package test

func main() {
	s := []int{1, 2, 3}
	s[0] = s[1]
	_ = s
}
`
	// Reference: no transform (int is a Copy type in Rust, no borrow conflict)
	//   s[0] = s[1] → s[0] = s[1]  (unchanged)
	want := "s[0] = s[1]"
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, want)
	assertNotContains(t, result, "_t0 :=")
}

// --- 3. Same variable as multiple function arguments ---

func TestCanonicalizeRust_SameVarMultipleArgs(t *testing.T) {
	src := `package test

func process(a, b string) string { return a + b }

func main() {
	x := "hello"
	r := process(x, x)
	_ = r
}
`
	// Reference: expected code after transform (Rust can't move x twice)
	//   process(x, x)
	// → _t0 := x
	//   process(_t0, x)  or  process(x, _t0)
	wantTemp := "_t0 :="
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
}

func TestCanonicalizeRust_SameVarMultipleArgs_CopyTypeSkipped(t *testing.T) {
	src := `package test

func add(a, b int) int { return a + b }

func main() {
	x := 42
	r := add(x, x)
	_ = r
}
`
	// Reference: no transform (int is a Copy type in Rust, no move conflict)
	//   add(x, x) → add(x, x)  (unchanged)
	want := "add(x, x)"
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, want)
	assertNotContains(t, result, "_t0 :=")
}

// --- 4. Same variable in binary expression with function calls ---

func TestCanonicalizeRust_BinaryExprSameVar(t *testing.T) {
	src := `package test

func consume(s string) string { return s }

func main() {
	x := "hello"
	r := consume(x) + consume(x)
	_ = r
}
`
	// Reference: expected code after transform (Rust can't move x into two calls)
	//   consume(x) + consume(x)
	// → _t0 := consume(x)
	//   r := _t0 + consume(x)  or similar extraction
	wantTemp := "_t0 :="
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
}

// --- 5. Nested function calls sharing non-Copy variable ---

func TestCanonicalizeRust_NestedCallSharing(t *testing.T) {
	src := `package test

func outer(a string, b string) string { return a + b }
func inner(s string) string { return s }

func main() {
	x := "hello"
	r := outer(x, inner(x))
	_ = r
}
`
	// Reference: expected code after transform (Rust can't move x into outer and inner)
	//   outer(x, inner(x))
	// → _t0 := inner(x)
	//   outer(x, _t0)
	wantTemp := "_t0 :="
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantTemp)
}

func TestCanonicalizeRust_NestedCallSharing_CopyTypeSkipped(t *testing.T) {
	src := `package test

func outer(a int, b int) int { return a + b }
func inner(n int) int { return n }

func main() {
	x := 42
	r := outer(x, inner(x))
	_ = r
}
`
	// Reference: no transform (int is a Copy type, no move conflict)
	//   outer(x, inner(x)) → outer(x, inner(x))  (unchanged)
	want := "outer(x, inner(x))"
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, want)
	assertNotContains(t, result, "_t0 :=")
}

// --- 6. String reuse after concatenation ---

func TestCanonicalizeRust_StringReuseAfterConcat(t *testing.T) {
	src := `package test

func main() {
	x := "hello"
	a := "a"
	b := "b"
	y := x + a
	z := x + b
	_ = y
	_ = z
}
`
	// Reference: expected code after transform (Rust moves x on first concat)
	//   y := x + a; z := x + b
	// → x_copy := x
	//   y := x_copy + a  (or x + a, with copy used for the other)
	//   z := x + b
	wantCopy := "x_copy :="
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantCopy)
}

func TestCanonicalizeRust_StringReuseAfterConcat_SkipWhenNoRust(t *testing.T) {
	src := `package test

func main() {
	x := "hello"
	a := "a"
	b := "b"
	y := x + a
	z := x + b
	_ = y
	_ = z
}
`
	// Reference: no transform (C++ doesn't have move semantics issues here)
	//   y := x + a; z := x + b → unchanged (no x_copy)
	result := runCanonicalize(t, src, BackendCpp)
	assertNotContains(t, result, "x_copy")
}

// --- 7. Multiple closures capturing same non-Copy variable ---

func TestCanonicalizeRust_MultiClosureSameVar(t *testing.T) {
	src := `package test

func use(s string) {}

func main() {
	x := "hello"
	fn1 := func() { use(x) }
	fn2 := func() { use(x) }
	fn1()
	fn2()
}
`
	// Reference: expected code after transform (Rust can't move x into two closures)
	//   fn1 := func() { use(x) }
	//   fn2 := func() { use(x) }
	// → fn1 := func() { use(x) }
	//   x_copy := x
	//   fn2 := func() { use(x_copy) }
	wantCopy := "x_copy :="
	result := runCanonicalize(t, src, BackendRust)
	assertValidGo(t, result)
	assertContains(t, result, wantCopy)
}

func TestCanonicalizeRust_MultiClosureSameVar_SkipWhenNoRust(t *testing.T) {
	src := `package test

func use(s string) {}

func main() {
	x := "hello"
	fn1 := func() { use(x) }
	fn2 := func() { use(x) }
	fn1()
	fn2()
}
`
	// Reference: no transform (C++ doesn't have closure move semantics)
	//   fn1, fn2 both capture x → unchanged (no x_copy)
	result := runCanonicalize(t, src, BackendCpp)
	assertNotContains(t, result, "x_copy")
}

// =============================================
// Named Return Lowering tests
// =============================================

func TestCanonicalizeNamedReturn_Basic(t *testing.T) {
	src := `package test

func f() (x int) {
	x = 42
	return
}
`
	// Reference: expected code after transform
	//   func f() (x int) { x = 42; return }
	// → func f() int { var x int; x = 42; return x }
	want := `func f() int {
	var x int
	x = 42
	return x
}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeNamedReturn_Multiple(t *testing.T) {
	src := `package test

func f() (x int, err error) {
	x = 42
	return
}
`
	// Reference: expected code after transform
	//   func f() (x int, err error) { x = 42; return }
	// → func f() (int, error) { var x int; var err error; x = 42; return x, err }
	want := `func f() (int, error) {
	var x int
	var err error
	x = 42
	return x, err
}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeNamedReturn_MixedBareAndExplicit(t *testing.T) {
	src := `package test

func f(flag bool) (x int) {
	if flag {
		return 99
	}
	x = 42
	return
}
`
	// Reference: expected code after transform
	//   Bare return → return x; explicit return 99 → unchanged
	want := `func f(flag bool) int {
	var x int
	if flag {
		return 99
	}
	x = 42
	return x
}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeNamedReturn_NoNamedReturns(t *testing.T) {
	src := `package test

func f() int {
	return 42
}
`
	// Reference: no transform needed (no named returns)
	want := `func f() int {
	return 42
}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, want)
	assertNotContains(t, result, "var")
}

func TestCanonicalizeNamedReturn_ExplicitReturnOnly(t *testing.T) {
	src := `package test

func f() (x int) {
	x = 42
	return x
}
`
	// Reference: var decl inserted, explicit return unchanged
	want := `func f() int {
	var x int
	x = 42
	return x
}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

func TestCanonicalizeNamedReturn_AlwaysRun(t *testing.T) {
	src := `package test

func f() (x int) {
	x = 42
	return
}
`
	// Reference: same transform as Basic, even with only Go backend
	want := `func f() int {
	var x int
	x = 42
	return x
}`
	// Should transform even with only one backend
	result := runCanonicalize(t, src, BackendGo)
	assertValidGo(t, result)
	assertContains(t, result, want)
}

// =============================================
// Iota Expansion tests
// =============================================

func TestCanonicalizeIota_Basic(t *testing.T) {
	src := `package test

const (
	A = iota
	B
	C
)

func main() {
	_ = A
	_ = B
	_ = C
}
`
	// Reference: expected code after transform
	//   const ( A = iota; B; C )
	// → const ( A int = 0; B int = 1; C int = 2 )
	wantConst := "const (\n\tA\tint\t= 0\n\tB\tint\t= 1\n\tC\tint\t= 2\n)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
	assertNotContains(t, result, "iota")
}

func TestCanonicalizeIota_Expression(t *testing.T) {
	src := `package test

const (
	A = 1 << iota
	B
	C
)

func main() {
	_ = A
	_ = B
	_ = C
}
`
	// Reference: expected code after transform
	//   const ( A = 1 << iota; B; C )
	// → const ( A int = 1; B int = 2; C int = 4 )
	wantConst := "const (\n\tA\tint\t= 1\n\tB\tint\t= 2\n\tC\tint\t= 4\n)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
}

func TestCanonicalizeIota_Offset(t *testing.T) {
	src := `package test

const (
	A = iota + 10
	B
	C
)

func main() {
	_ = A
	_ = B
	_ = C
}
`
	// Reference: expected code after transform
	//   const ( A = iota + 10; B; C )
	// → const ( A int = 10; B int = 11; C int = 12 )
	wantConst := "const (\n\tA\tint\t= 10\n\tB\tint\t= 11\n\tC\tint\t= 12\n)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
}

func TestCanonicalizeIota_NamedType(t *testing.T) {
	src := `package test

type Color int

const (
	Red Color = iota
	Green
	Blue
)

func main() {
	_ = Red
	_ = Green
	_ = Blue
}
`
	// Reference: expected code after transform (named type preserved)
	//   const ( Red Color = iota; Green; Blue )
	// → const ( Red Color = 0; Green Color = 1; Blue Color = 2 )
	wantConst := "const (\n\tRed\tColor\t= 0\n\tGreen\tColor\t= 1\n\tBlue\tColor\t= 2\n)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
}

func TestCanonicalizeIota_NonIotaConst(t *testing.T) {
	src := `package test

const (
	X int = 42
	Y int = 99
)

func main() {
	_ = X
	_ = Y
}
`
	// Reference: no transform needed (no iota) — output unchanged
	wantConst := "const (\n\tX\tint\t= 42\n\tY\tint\t= 99\n)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
}

func TestCanonicalizeIota_SingleConst(t *testing.T) {
	src := `package test

const X = iota

func main() {
	_ = X
}
`
	// Reference: expected code after transform
	//   const X = iota
	// → const X int = 0
	// Note: single const (not grouped) uses spaces, not tabs
	wantConst := "const X int = 0"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantConst)
	assertNotContains(t, result, "iota")
}

// =============================================
// Variadic Lowering tests
// =============================================

func TestCanonicalizeVariadic_IndividualArgs(t *testing.T) {
	src := `package test

func sum(args ...int) int {
	total := 0
	for _, a := range args {
		total += a
	}
	return total
}

func main() {
	r := sum(1, 2, 3)
	_ = r
}
`
	// Reference: expected code after transform
	//   func sum(args ...int) → func sum(args []int)
	//   sum(1, 2, 3)          → sum([]int{1, 2, 3})
	wantDecl := "func sum(args []int) int {"
	wantCall := "sum([]int{1, 2, 3})"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantDecl)
	assertContains(t, result, wantCall)
	assertNotContains(t, result, "...int")
}

func TestCanonicalizeVariadic_Spread(t *testing.T) {
	src := `package test

func sum(args ...int) int {
	total := 0
	for _, a := range args {
		total += a
	}
	return total
}

func main() {
	s := []int{1, 2, 3}
	r := sum(s...)
	_ = r
}
`
	// Reference: expected code after transform
	//   func sum(args ...int) → func sum(args []int)
	//   sum(s...)              → sum(s)   (spread removed, slice passed directly)
	wantDecl := "func sum(args []int) int {"
	wantCall := "sum(s)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantDecl)
	assertContains(t, result, wantCall)
	assertNotContains(t, result, "...")
}

func TestCanonicalizeVariadic_ZeroArgs(t *testing.T) {
	src := `package test

func sum(args ...int) int {
	return 0
}

func main() {
	r := sum()
	_ = r
}
`
	// Reference: expected code after transform
	//   func sum(args ...int) → func sum(args []int)
	//   sum()                  → sum([]int{})   (empty slice inserted)
	wantDecl := "func sum(args []int) int {"
	// Note: printer may split []int{} across lines; use partial match
	wantCall := "sum([]int{}"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantDecl)
	assertContains(t, result, wantCall)
}

func TestCanonicalizeVariadic_WithPrefix(t *testing.T) {
	src := `package test

func log(prefix string, args ...int) {
	_ = prefix
	_ = args
}

func main() {
	log("test", 1, 2)
}
`
	// Reference: expected code after transform
	//   func log(prefix string, args ...int) → func log(prefix string, args []int)
	//   log("test", 1, 2)                    → log("test", []int{1, 2})
	wantDecl := "func log(prefix string, args []int)"
	wantCall := `log("test", []int{1, 2})`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantDecl)
	assertContains(t, result, wantCall)
}

func TestCanonicalizeVariadic_ZeroWithPrefix(t *testing.T) {
	src := `package test

func log(prefix string, args ...int) {
	_ = prefix
	_ = args
}

func main() {
	log("test")
}
`
	// Reference: expected code after transform
	//   func log(prefix string, args ...int) → func log(prefix string, args []int)
	//   log("test")                           → log("test", []int{})
	wantDecl := "func log(prefix string, args []int)"
	// Note: printer may split across lines; use partial match
	wantCall := `"test", []int{}`
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantDecl)
	assertContains(t, result, wantCall)
}

func TestCanonicalizeVariadic_BuiltinNotTransformed(t *testing.T) {
	src := `package test

func main() {
	s := []int{1}
	s = append(s, 2, 3)
	_ = s
}
`
	// Reference: no transform needed (built-in variadics are not lowered)
	//   append(s, 2, 3) → append(s, 2, 3)  (unchanged)
	wantCall := "append(s, 2, 3)"
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, wantCall)
}

// =============================================
// Make Capacity Stripping tests
// =============================================

func TestCanonicalizeMakeCapacity_SliceWithCap(t *testing.T) {
	src := `package test

func main() {
	s := make([]int, 5, 10)
	_ = s
}
`
	// Reference: expected code after transform
	//   make([]int, 5, 10) → make([]int, 5)
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "make([]int, 5)")
	assertNotContains(t, result, "10")
}

func TestCanonicalizeMakeCapacity_SliceZeroCap(t *testing.T) {
	src := `package test

func main() {
	s := make([]string, 0, 100)
	_ = s
}
`
	// Reference: expected code after transform
	//   make([]string, 0, 100) → make([]string, 0)
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "make([]string, 0)")
	assertNotContains(t, result, "100")
}

func TestCanonicalizeMakeCapacity_MapUnchanged(t *testing.T) {
	src := `package test

func main() {
	m := make(map[string]int)
	_ = m
}
`
	// Reference: no transform (maps don't have a capacity arg)
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "make(map[string]int)")
}

func TestCanonicalizeMakeCapacity_SliceNoCap(t *testing.T) {
	src := `package test

func main() {
	s := make([]int, 5)
	_ = s
}
`
	// Reference: no transform (already 2-arg form)
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "make([]int, 5)")
}

// =============================================
// Integer Range Lowering tests
// =============================================

func TestCanonicalizeIntegerRange_Basic(t *testing.T) {
	src := `package test

func main() {
	for i := range 10 {
		_ = i
	}
}
`
	// Reference: expected code after transform
	//   for i := range 10 → for i := 0; i < 10; i++
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "i := 0; i < 10; i++")
	assertNotContains(t, result, "range 10")
}

func TestCanonicalizeIntegerRange_Variable(t *testing.T) {
	src := `package test

func main() {
	n := 5
	for i := range n {
		_ = i
	}
}
`
	// Reference: expected code after transform
	//   for i := range n → for i := 0; i < n; i++
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "i := 0; i < n; i++")
	assertNotContains(t, result, "range n")
}

func TestCanonicalizeIntegerRange_NoKey(t *testing.T) {
	src := `package test

func main() {
	for range 5 {
		println("hello")
	}
}
`
	// Reference: expected code after transform
	//   for range 5 → for _t0 := 0; _t0 < 5; _t0++
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "< 5")
	assertNotContains(t, result, "range 5")
}

func TestCanonicalizeIntegerRange_SliceNotTransformed(t *testing.T) {
	src := `package test

func main() {
	s := []int{1, 2, 3}
	for i, v := range s {
		_ = i
		_ = v
	}
}
`
	// Reference: no transform (slice range stays as range)
	result := runCanonicalize(t, src, BackendAll)
	assertValidGo(t, result)
	assertContains(t, result, "range s")
}
