package main

// This file contains all supported Go language constructs that compile
// successfully across all backends (C++, C#, Rust).
//
// UNSUPPORTED CONSTRUCTS (not included in this file):
//
// 1. if slice == nil - Nil comparison for slices
//    C++ std::vector cannot be compared to nullptr
//
// 2. len(string) - String length
//    C++ backend uses std::size() which doesn't work on C-style strings
//
// 3. iota - Constant enumeration
//    Not yet implemented
//
// 4. fmt.Sprintf - String formatting
//    Rust backend has type mismatch issues with string_format2
//
// 5. for _, x := range []int{1,2,3} - Range over inline slice literal
//    Rust backend generates malformed code
//
// 6. []interface{} - Slice of empty interface (any type)
//    Not supported across backends
//
// SUPPORTED WITH LIMITATIONS:
// - interface{} (empty interface) - works for assignment, no type assertions

import (
	"alltests/types"
	"fmt"
)

// Struct type declaration with slice field
type Composite struct {
	a []int
}

// Struct with multiple field types
type Person struct {
	name string
	age  int
}

// Basic function with single return value
func testBasicConstructs() int8 {
	testSliceOperations()
	testLoopConstructs()
	testBooleanLogic()
	return 5
}

// Function with multiple return values
func testFunctionCalls() (int16, int16) {
	return testFunctionVariables()
}

// Slice operations: nil slice, len, indexing, struct field access
func testSliceOperations() {
	var a []int
	c := Composite{}

	// Slice literal with int type (from slice test)
	intSlice := []int{1, 2, 3}
	fmt.Println(len(intSlice))

	if len(a) == 0 {
	} else {
		if a[0] == 0 {
			a[0] = 1
		}
	}

	if len(c.a) == 0 {
	}
}

// Loop constructs: C-style for, range for, while-style
func testLoopConstructs() {
	var a []int

	// C-style for loop
	// @test cpp="for (auto x = 0; x < 10; x++)" cs="for (var x = 0; (x < 10 ); x++)" rust="for x in 0..10"
	for x := 0; x < 10; x++ {
		if !(len(a) == 0) {
		} else if len(a) == 0 {
		}
	}

	// Range-based for loop with blank identifier
	for _, x := range a {
		if x == 0 {
		}
	}

	// Range-based for loop with index and value
	// @test cpp="for (size_t i = 0; i < nums2.size(); i++)" cs="for (int i = 0; i < nums2.Count; i++)" rust="for (i, v) in nums2.clone().iter().enumerate()"
	nums2 := []int{10, 20, 30}
	for i, v := range nums2 {
		fmt.Println(i)
		fmt.Println(v)
	}

	// While-style loop
	// @test cpp="for (; counter < 5;)" cs="for (; (counter < 5 );)" rust="while (counter < 5)"
	counter := 0
	for counter < 5 {
		counter++
	}
	fmt.Println(counter)

	// Infinite loop with break
	// @test cpp="for (;;)" cs="for (;;)" rust="loop"
	counter2 := 0
	for {
		counter2++
		if counter2 >= 3 {
			break
		}
	}
	fmt.Println(counter2)

	// Continue statement
	sum := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
		sum += i
	}
	fmt.Println(sum)

	// Index-only range loop
	nums := []int{10, 20, 30}
	for i := range nums {
		fmt.Println(i)
	}

	// Step by 2: i += 2
	// @test cpp="for (auto i = 0; i < 10; i += 2)" cs="for (var i = 0; (i < 10 ); i += 2)" rust="for i in (0..10).step_by(2)"
	sumStep := 0
	for i := 0; i < 10; i += 2 {
		sumStep += i // 0 + 2 + 4 + 6 + 8 = 20
	}
	fmt.Println(sumStep)

	// Decrement loop: i--
	// @test cpp="for (auto i = 5; i > 0; i--)" cs="for (var i = 5; (i > 0 ); i--)" rust="for i in ((0 + 1)..=5).rev()"
	sumDecr := 0
	for i := 5; i > 0; i-- {
		sumDecr += i // 5 + 4 + 3 + 2 + 1 = 15
	}
	fmt.Println(sumDecr)

	// Inclusive range: i <= n
	// @test cpp="for (auto i = 1; i <= 5; i++)" cs="for (var i = 1; (i <= 5 ); i++)" rust="for i in 1..=5"
	sumIncl := 0
	for i := 1; i <= 5; i++ {
		sumIncl += i // 1 + 2 + 3 + 4 + 5 = 15
	}
	fmt.Println(sumIncl)

	// Decrement with inclusive: i >= 0
	// @test cpp="for (auto i = 3; i >= 0; i--)" cs="for (var i = 3; (i >= 0 ); i--)" rust="for i in (0..=3).rev()"
	sumDecrIncl := 0
	for i := 3; i >= 0; i-- {
		sumDecrIncl += i // 3 + 2 + 1 + 0 = 6
	}
	fmt.Println(sumDecrIncl)

	// Step by 3 decrement: i -= 3
	// @test cpp="for (auto i = 9; i > 0; i -= 3)" cs="for (var i = 9; (i > 0 ); i -= 3)" rust="for i in ((0 + 1)..=9).rev().step_by(3)"
	sumDecrStep := 0
	for i := 9; i > 0; i -= 3 {
		sumDecrStep += i // 9 + 6 + 3 = 18
	}
	fmt.Println(sumDecrStep)

	// Compound condition with && (cannot be converted to simple range)
	// @test rust="while ((i < 10) && (i < limit))"
	limit := 5
	sumCompound := 0
	for i := 0; i < 10 && i < limit; i++ {
		sumCompound += i // 0 + 1 + 2 + 3 + 4 = 10
	}
	fmt.Println(sumCompound)

	// Compound condition with || (cannot be converted to simple range)
	// @test rust="while ((i < 3) || flag)"
	sumOr := 0
	flag := false
	for i := 0; i < 3 || flag; i++ {
		sumOr += i // 0 + 1 + 2 = 3
		if i >= 2 {
			flag = false
		}
	}
	fmt.Println(sumOr)

	// Compound condition with slice length check (common pattern)
	// @test rust="while ((i < maxItems) && (i < len(&items.clone())))"
	items := []int{10, 20, 30}
	maxItems := 5
	sumItems := 0
	for i := 0; i < maxItems && i < len(items); i++ {
		sumItems += items[i] // 10 + 20 + 30 = 60
	}
	fmt.Println(sumItems)

	// Multiple compound conditions
	// @test rust="while (((i < 10) && (i < limit2)) && (sumMulti < 20))"
	limit2 := 8
	sumMulti := 0
	for i := 0; i < 10 && i < limit2 && sumMulti < 20; i++ {
		sumMulti += i // stops when sumMulti >= 20
	}
	fmt.Println(sumMulti)
}

// Boolean logic: not operator, boolean literals
func testBooleanLogic() {
	b := false
	if !b {
	}

	c := true
	if c {
	}
}

// Function types: slice of functions, closures, calling through variables
func testFunctionVariables() (int16, int16) {
	x := []func(int, int){
		func(a int, b int) {
			fmt.Println(a)
			fmt.Println(b)
		},
	}

	f := x[0]
	f(10, 20)
	x[0](20, 30)

	if len(x) == 0 {
	}

	return 10, 20
}

// Sink function for consuming values
func sink(p int8) {
}

// Empty slice and slice with values initialization
func testArrayInitialization() {
	a := []int8{}
	if len(a) == 0 {
	}

	b := []int8{1, 2, 3}
	if len(b) == 0 {
	}
}

// Slice expressions: slicing with start index
func testSliceExpressions() {
	a := []int8{1, 2, 3}

	// Slice from index to end
	b := a[1:]
	if len(b) == 0 {
	}

	// Slice from start to index
	c := a[:2]
	if len(c) == 0 {
	}

	// Slice with both bounds
	d := a[1:2]
	if len(d) == 0 {
	}
}

// Variable declarations: var, short declaration, multiple on one line
func testVariableDeclarations() {
	var a int8
	var b, c int16

	a = 1
	a = a + 5
	d := 10

	sink(a)
	if b == 0 {
	}
	if c == 0 {
	}
	if d == 10 {
	}
}

// Arithmetic operators
func testArithmeticOperators() {
	a := 10
	b := 3

	sum := a + b
	diff := a - b
	prod := a * b
	quot := a / b
	rem := a % b

	fmt.Println(sum)
	fmt.Println(diff)
	fmt.Println(prod)
	fmt.Println(quot)
	fmt.Println(rem)
}

// Comparison operators
func testComparisonOperators() {
	a := 10
	b := 20

	if a == b {
	}
	if a != b {
	}
	if a < b {
	}
	if a > b {
	}
	if a <= b {
	}
	if a >= b {
	}
}

// Logical operators
func testLogicalOperators() {
	a := true
	b := false

	if a && b {
	}
	if a || b {
	}
	if !a {
	}
}

// Assignment operators
func testAssignmentOperators() {
	a := 10
	a = 20
	a += 5
	a -= 3

	fmt.Println(a)
}

// Increment and decrement
func testIncrementDecrement() {
	a := 0
	a++
	a--
	fmt.Println(a)
}

// String operations
func testStringOperations() {
	s := "hello"
	fmt.Println(s)
}

// Print functions
func testPrintFunctions() {
	// Print with newline
	fmt.Println("Hello")
	fmt.Println(42)
	fmt.Println()

	// Print without newline
	fmt.Print("World")
	fmt.Print("\n")

	// Printf with format specifiers
	fmt.Printf("%d\n", 100)
	fmt.Printf("%s\n", "test")
}

// Type conversions
func testTypeConversions() {
	a := 65
	b := int8(a)
	sink(b)
}

// Append operation
func testAppend() {
	a := []int{}
	a = append(a, 1)
	a = append(a, 2)
	a = append(a, 3)
	fmt.Println(len(a))
}

// Struct initialization
func testStructInitialization() {
	// Empty struct
	c := Composite{}
	if len(c.a) == 0 {
	}

	// Struct with field values
	p := Person{name: "Alice", age: 30}
	fmt.Println(p.name)
	fmt.Println(p.age)
}

// Nested if statements
func testNestedIf() {
	a := 10
	b := 20

	if a == 10 {
		if b == 20 {
			fmt.Println("nested")
		}
	}
}

// Test int32, int64 types (from iceberg)
func testInt32Int64Types() {
	var a int32
	var b int64

	a = 100
	b = 200

	fmt.Println(a)
	fmt.Println(b)

	// Struct with int32/int64 fields
	record := types.DataRecord{
		ID:          1,
		Size:        1024,
		Count:       10,
		SequenceNum: 999,
	}
	fmt.Println(record.ID)
	fmt.Println(record.Size)
}

// Test type aliases (from substrait)
func testTypeAliases() {
	var kind types.ExprKind
	kind = types.ExprLiteral

	if kind == types.ExprLiteral {
		fmt.Println("literal")
	}
	if kind == types.ExprColumn {
		fmt.Println("column")
	}

	var relKind types.RelNodeKind
	relKind = types.RelOpScan
	if relKind == types.RelOpScan {
		fmt.Println("scan")
	}
}

// Test fmt.Printf with multiple arguments (from substrait)
func testPrintfMultipleArgs() {
	fmt.Printf("a=%d, b=%d\n", 10, 20)
	fmt.Printf("name=%s, value=%d\n", "test", 100)
}

// Test zero-value struct declaration (from substrait)
func testZeroValueStruct() {
	var plan types.Plan
	plan.Literals = []string{}
	plan.Root = 0
	fmt.Println(plan.Root)
	fmt.Println(len(plan.Literals))
}

// Test function returning modified struct
func testReturnModifiedStruct() {
	var plan types.Plan
	plan.Literals = []string{}

	idx := 0
	plan, idx = types.AddLiteralToPlan(plan, "first")
	plan, idx = types.AddLiteralToPlan(plan, "second")

	fmt.Println(idx)
	fmt.Println(len(plan.Literals))
}

// Test bool field in struct (from iceberg)
func testBoolFieldInStruct() {
	f := types.Field{
		ID:       1,
		Name:     "column1",
		Required: true,
	}
	fmt.Println(f.ID)
	fmt.Println(f.Name)
	if f.Required {
		fmt.Println("required")
	}

	f2 := types.Field{
		ID:       2,
		Name:     "column2",
		Required: false,
	}
	if !f2.Required {
		fmt.Println("optional")
	}
}

// Test nested struct field (from iceberg)
func testNestedStructField() {
	stats := types.ColumnStats{NullCount: 100}
	dataFile := types.DataFile{
		FilePath:    "/path/to/file",
		RecordCount: 1000,
		Stats:       stats,
	}
	entry := types.ManifestEntry{
		Status:    1,
		DataFileF: dataFile,
	}

	fmt.Println(entry.Status)
	fmt.Println(entry.DataFileF.FilePath)
	fmt.Println(entry.DataFileF.RecordCount)
	fmt.Println(entry.DataFileF.Stats.NullCount)
}

// Test multi-package import (from iceberg pattern)
func testMultiPackageImport() {
	// Use types from the types package
	record := types.DataRecord{
		ID:          42,
		Size:        2048,
		Count:       5,
		SequenceNum: 100,
	}
	types.LoadData(record)

	// Use function from types package
	var plan types.Plan
	plan.Literals = []string{}
	idx := 0
	plan, idx = types.AddLiteralToPlan(plan, "value1")
	plan, idx = types.AddLiteralToPlan(plan, "value2")
	fmt.Println(idx)
	fmt.Println(len(plan.Literals))

	// Use constants from types package
	if types.ExprLiteral == 0 {
		fmt.Println("literal is 0")
	}

	// Use nested struct from types package
	entry := types.ManifestEntry{
		Status: 1,
		DataFileF: types.DataFile{
			FilePath:    "/data/file.parquet",
			RecordCount: 500,
			Stats:       types.ColumnStats{NullCount: 10},
		},
	}
	fmt.Println(entry.DataFileF.FilePath)
}

// Complete language feature test
func testCompleteLanguageFeatures() {
	var a int8
	var b, c int16

	a = 1
	a = a + 5
	d := 10

	a = testBasicConstructs()
	b, c = testFunctionCalls()

	if (a == 1) && (b == 10) {
		a = 2
		var aa int8
		aa = testBasicConstructs()
		sink(aa)

		if a == 5 {
			a = 10
		}
	} else {
		a = 3
	}

	if b == 10 {
	}
	if c == 20 {
	}
	if d == 10 {
	}
}

// Test empty interface (interface{})
func testEmptyInterface() {
	var x interface{}
	x = 1
	x = "hello"
	x = true
	x = 3.14

	// Assign to another interface{} variable
	var y interface{}
	y = x

	// Suppress unused variable warning by assigning back
	x = y

	fmt.Println("interface{} test passed")
}

// Test type assertions on interface{}
func testTypeAssertions() {
	var a interface{}
	a = 42
	intVal := a.(int)
	fmt.Println(intVal)

	a = "world"
	strVal := a.(string)
	fmt.Println(strVal)

	a = true
	boolVal := a.(bool)
	fmt.Println(boolVal)

	a = 2.71
	floatVal := a.(float64)
	fmt.Println(floatVal)

	fmt.Println("type assertions passed")
}

// Helper: sum all elements in a slice and print the result
func printSliceSum(s []int) {
	total := 0
	i := 0
	for i < len(s) {
		total = total + s[i]
		i = i + 1
	}
	fmt.Printf("sum: %d\n", total)
}

// Nested condition-only for loops (while-style)
// Tests that nested "for cond { for cond { } }" generates correct Rust
func testNestedWhileLoops() {
	// Nested condition-only for loops
	outer := 0
	innerTotal := 0
	for outer < 3 {
		j := 0
		for j < 4 {
			innerTotal = innerTotal + 1
			j = j + 1
		}
		outer = outer + 1
	}
	fmt.Printf("nested while: %d\n", innerTotal) // 3 * 4 = 12

	// Condition-only loop nested inside traditional for loop
	sum2 := 0
	for i := 0; i < 3; i++ {
		k := 0
		for k < 2 {
			sum2 = sum2 + 1
			k = k + 1
		}
	}
	fmt.Printf("traditional outer, while inner: %d\n", sum2) // 3 * 2 = 6

	// Three levels of nesting: while > while > while
	total3 := 0
	a := 0
	for a < 2 {
		b := 0
		for b < 2 {
			c := 0
			for c < 2 {
				total3 = total3 + 1
				c = c + 1
			}
			b = b + 1
		}
		a = a + 1
	}
	fmt.Printf("triple nested while: %d\n", total3) // 2 * 2 * 2 = 8
}

// Inline composite literal as function argument
// Tests that func([]int{1,2,3}) generates correct Rust
func testInlineCompositeLitArg() {
	// Pass inline composite literal to a function
	printSliceSum([]int{10, 20, 30}) // sum: 60

	// Pass inline composite literal with single element
	printSliceSum([]int{42}) // sum: 42

	// Pass inline composite literal with many elements
	printSliceSum([]int{1, 2, 3, 4, 5}) // sum: 15
}

// Helper: set a value in a map and return it (tests map as param + return)
func setMapValue(m map[string]int, key string, value int) map[string]int {
	m[key] = value
	return m
}

// Helper: get map length (tests map as param with int return)
func getMapLen(m map[string]int) int {
	return len(m)
}

// Test map as function parameter and return value
func testMapAsParameter() {
	m := make(map[string]int)
	m = setMapValue(m, "hello", 1)
	m = setMapValue(m, "world", 2)
	fmt.Println(getMapLen(m))
	fmt.Println(m["hello"])
	fmt.Println(m["world"])
}

// Test map operations: make, get, set, delete, len with string/int/bool keys
func testMapOperations() {
	// String keys
	m := make(map[string]int)
	m["hello"] = 1
	m["world"] = 2
	fmt.Println(m["hello"])
	fmt.Println(m["world"])
	fmt.Println(len(m))
	delete(m, "hello")
	fmt.Println(len(m))

	// Int keys
	m2 := make(map[int]string)
	m2[42] = "answer"
	m2[7] = "lucky"
	fmt.Println(m2[42])
	fmt.Println(len(m2))

	// Bool keys
	m3 := make(map[bool]int)
	m3[true] = 1
	m3[false] = 0
	fmt.Println(m3[true])
	fmt.Println(m3[false])
}

func testNilMap() {
	var m map[string]int
	fmt.Println(len(m))
	m = make(map[string]int)
	m["x"] = 10
	fmt.Println(m["x"])
	fmt.Println(len(m))
}

func testMapKeyTypes() {
	m1 := make(map[int64]string)
	m1[100] = "hundred"
	fmt.Println(m1[100])
	fmt.Println(len(m1))

	m2 := make(map[float64]int)
	m2[3.14] = 314
	fmt.Println(m2[3.14])
	fmt.Println(len(m2))

	m3 := make(map[int32]string)
	m3[42] = "answer"
	fmt.Println(m3[42])
	fmt.Println(len(m3))
}

type MapTestStruct struct {
	Name  string
	Value int
}

func testMapStructValue() {
	m := make(map[string]MapTestStruct)
	m["first"] = MapTestStruct{Name: "hello", Value: 42}
	s := m["first"]
	fmt.Println(s.Name)
	fmt.Println(s.Value)
	fmt.Println(len(m))
}

func testMapCommaOk() {
	m := make(map[string]int)
	m["hello"] = 42

	val, ok := m["hello"]
	fmt.Println(val)
	fmt.Println(ok)

	val2, ok2 := m["missing"]
	fmt.Println(val2)
	fmt.Println(ok2)

	fmt.Println(len(m))
}

func testTypeAssertCommaOk() {
	fmt.Println("=== Type Assert Comma-Ok ===")
	var x interface{}
	x = 42
	val, ok := x.(int)
	fmt.Println(val)
	fmt.Println(ok)

	val2, ok2 := x.(string)
	fmt.Println(val2)
	fmt.Println(ok2)
}

func testIfInitCommaOk() {
	fmt.Println("=== If-Init Comma-Ok ===")
	m := make(map[string]int)
	m["hello"] = 42

	if val, ok := m["hello"]; ok {
		fmt.Println(val)
	}
	if val, ok := m["missing"]; ok {
		fmt.Println(val)
	} else {
		fmt.Println("not found")
	}

	var x interface{}
	x = 10
	if val, ok := x.(int); ok {
		fmt.Println(val)
	}
}

func main() {
	fmt.Println("=== All Language Constructs Test ===")

	testCompleteLanguageFeatures()
	testArrayInitialization()
	testSliceExpressions()
	testVariableDeclarations()
	testArithmeticOperators()
	testComparisonOperators()
	testLogicalOperators()
	testAssignmentOperators()
	testIncrementDecrement()
	testStringOperations()
	testPrintFunctions()
	testTypeConversions()
	testAppend()
	testStructInitialization()
	testNestedIf()
	testInt32Int64Types()
	testTypeAliases()
	testPrintfMultipleArgs()
	testZeroValueStruct()
	testReturnModifiedStruct()
	testBoolFieldInStruct()
	testNestedStructField()
	testMultiPackageImport()
	testEmptyInterface()
	testTypeAssertions()
	testNestedWhileLoops()
	testInlineCompositeLitArg()
	testMapOperations()
	testMapAsParameter()
	testNilMap()
	testMapKeyTypes()
	testMapStructValue()
	testMapCommaOk()
	testTypeAssertCommaOk()
	testIfInitCommaOk()

	fmt.Println("=== Done ===")
}
