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
	if len(intSlice) == 3 {
		fmt.Println("PASS: slice literal len")
	} else {
		panic("FAIL: slice literal len")
	}

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
	if counter == 5 {
		fmt.Println("PASS: while loop")
	} else {
		panic("FAIL: while loop")
	}

	// Infinite loop with break
	// @test cpp="for (;;)" cs="for (;;)" rust="loop"
	counter2 := 0
	for {
		counter2++
		if counter2 >= 3 {
			break
		}
	}
	if counter2 == 3 {
		fmt.Println("PASS: infinite loop break")
	} else {
		panic("FAIL: infinite loop break")
	}

	// Continue statement
	sum := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
		sum += i
	}
	if sum == 25 {
		fmt.Println("PASS: continue statement")
	} else {
		panic("FAIL: continue statement")
	}

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
	if sumStep == 20 {
		fmt.Println("PASS: step by 2")
	} else {
		panic("FAIL: step by 2")
	}

	// Decrement loop: i--
	// @test cpp="for (auto i = 5; i > 0; i--)" cs="for (var i = 5; (i > 0 ); i--)" rust="for i in ((0 + 1)..=5).rev()"
	sumDecr := 0
	for i := 5; i > 0; i-- {
		sumDecr += i // 5 + 4 + 3 + 2 + 1 = 15
	}
	if sumDecr == 15 {
		fmt.Println("PASS: decrement loop")
	} else {
		panic("FAIL: decrement loop")
	}

	// Inclusive range: i <= n
	// @test cpp="for (auto i = 1; i <= 5; i++)" cs="for (var i = 1; (i <= 5 ); i++)" rust="for i in 1..=5"
	sumIncl := 0
	for i := 1; i <= 5; i++ {
		sumIncl += i // 1 + 2 + 3 + 4 + 5 = 15
	}
	if sumIncl == 15 {
		fmt.Println("PASS: inclusive range")
	} else {
		panic("FAIL: inclusive range")
	}

	// Decrement with inclusive: i >= 0
	// @test cpp="for (auto i = 3; i >= 0; i--)" cs="for (var i = 3; (i >= 0 ); i--)" rust="for i in (0..=3).rev()"
	sumDecrIncl := 0
	for i := 3; i >= 0; i-- {
		sumDecrIncl += i // 3 + 2 + 1 + 0 = 6
	}
	if sumDecrIncl == 6 {
		fmt.Println("PASS: decrement inclusive")
	} else {
		panic("FAIL: decrement inclusive")
	}

	// Step by 3 decrement: i -= 3
	// @test cpp="for (auto i = 9; i > 0; i -= 3)" cs="for (var i = 9; (i > 0 ); i -= 3)" rust="for i in ((0 + 1)..=9).rev().step_by(3)"
	sumDecrStep := 0
	for i := 9; i > 0; i -= 3 {
		sumDecrStep += i // 9 + 6 + 3 = 18
	}
	if sumDecrStep == 18 {
		fmt.Println("PASS: decrement step 3")
	} else {
		panic("FAIL: decrement step 3")
	}

	// Compound condition with && (cannot be converted to simple range)
	// @test rust="while ((i < 10) && (i < limit))"
	limit := 5
	sumCompound := 0
	for i := 0; i < 10 && i < limit; i++ {
		sumCompound += i // 0 + 1 + 2 + 3 + 4 = 10
	}
	if sumCompound == 10 {
		fmt.Println("PASS: compound condition")
	} else {
		panic("FAIL: compound condition")
	}

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
	if sumOr == 3 {
		fmt.Println("PASS: compound or condition")
	} else {
		panic("FAIL: compound or condition")
	}

	// Compound condition with slice length check (common pattern)
	// @test rust="while ((i < maxItems) && (i < len(&items.clone())))"
	items := []int{10, 20, 30}
	maxItems := 5
	sumItems := 0
	for i := 0; i < maxItems && i < len(items); i++ {
		sumItems += items[i] // 10 + 20 + 30 = 60
	}
	if sumItems == 60 {
		fmt.Println("PASS: slice length check")
	} else {
		panic("FAIL: slice length check")
	}

	// Multiple compound conditions
	// @test rust="while (((i < 10) && (i < limit2)) && (sumMulti < 20))"
	limit2 := 8
	sumMulti := 0
	for i := 0; i < 10 && i < limit2 && sumMulti < 20; i++ {
		sumMulti += i // stops when sumMulti >= 20
	}
	if sumMulti == 21 {
		fmt.Println("PASS: multiple compound conditions")
	} else {
		panic("FAIL: multiple compound conditions")
	}
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

	if sum == 13 {
		fmt.Println("PASS: arithmetic sum")
	} else {
		panic("FAIL: arithmetic sum")
	}
	if diff == 7 {
		fmt.Println("PASS: arithmetic diff")
	} else {
		panic("FAIL: arithmetic diff")
	}
	if prod == 30 {
		fmt.Println("PASS: arithmetic prod")
	} else {
		panic("FAIL: arithmetic prod")
	}
	if quot == 3 {
		fmt.Println("PASS: arithmetic quot")
	} else {
		panic("FAIL: arithmetic quot")
	}
	if rem == 1 {
		fmt.Println("PASS: arithmetic rem")
	} else {
		panic("FAIL: arithmetic rem")
	}
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

	if a == 22 {
		fmt.Println("PASS: assignment operators")
	} else {
		panic("FAIL: assignment operators")
	}
}

// Increment and decrement
func testIncrementDecrement() {
	a := 0
	a++
	a--
	if a == 0 {
		fmt.Println("PASS: increment decrement")
	} else {
		panic("FAIL: increment decrement")
	}
}

// String operations
func testStringOperations() {
	s := "hello"
	if s == "hello" {
		fmt.Println("PASS: string operations")
	} else {
		panic("FAIL: string operations")
	}
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
	if len(a) == 3 {
		fmt.Println("PASS: append len")
	} else {
		panic("FAIL: append len")
	}
}

// Test nil slice declaration followed by append
// This is a common Go pattern: var slice []Type; slice = append(slice, item)
func testNilSliceAppend() {
	// Test 1: Basic nil slice with append
	var nilSlice []int
	nilSlice = append(nilSlice, 1)
	nilSlice = append(nilSlice, 2)
	nilSlice = append(nilSlice, 3)
	if len(nilSlice) == 3 && nilSlice[0] == 1 && nilSlice[1] == 2 && nilSlice[2] == 3 {
		fmt.Println("PASS: nil slice append")
	} else {
		panic("FAIL: nil slice append")
	}

	// Test 2: Nil string slice with append
	var strSlice []string
	strSlice = append(strSlice, "hello")
	strSlice = append(strSlice, "world")
	if len(strSlice) == 2 && strSlice[0] == "hello" && strSlice[1] == "world" {
		fmt.Println("PASS: nil string slice append")
	} else {
		panic("FAIL: nil string slice append")
	}

	// Test 3: make with zero capacity
	makeSlice := make([]int, 0)
	makeSlice = append(makeSlice, 10)
	makeSlice = append(makeSlice, 20)
	if len(makeSlice) == 2 && makeSlice[0] == 10 && makeSlice[1] == 20 {
		fmt.Println("PASS: make zero capacity append")
	} else {
		panic("FAIL: make zero capacity append")
	}

	// Test 4: Nil slice in loop
	var loopSlice []int
	for i := 0; i < 5; i++ {
		loopSlice = append(loopSlice, i*2)
	}
	if len(loopSlice) == 5 && loopSlice[0] == 0 && loopSlice[4] == 8 {
		fmt.Println("PASS: nil slice loop append")
	} else {
		panic("FAIL: nil slice loop append")
	}
}

// Struct with slice field that starts nil
type SliceContainer struct {
	Items []int
	Names []string
}

// Test struct with nil slice fields and append
func testStructNilSliceField() {
	// Test 1: Struct with nil slice field
	container := SliceContainer{}
	container.Items = append(container.Items, 1)
	container.Items = append(container.Items, 2)
	if len(container.Items) == 2 && container.Items[0] == 1 && container.Items[1] == 2 {
		fmt.Println("PASS: struct nil slice field append")
	} else {
		panic("FAIL: struct nil slice field append")
	}

	// Test 2: Struct with multiple nil slice fields
	container.Names = append(container.Names, "Alice")
	container.Names = append(container.Names, "Bob")
	if len(container.Names) == 2 && container.Names[0] == "Alice" {
		fmt.Println("PASS: struct multiple nil slice fields")
	} else {
		panic("FAIL: struct multiple nil slice fields")
	}

	// Test 3: Struct initialized with empty slice literals
	container2 := SliceContainer{
		Items: []int{},
		Names: []string{},
	}
	container2.Items = append(container2.Items, 100)
	if len(container2.Items) == 1 && container2.Items[0] == 100 {
		fmt.Println("PASS: struct empty slice literal field")
	} else {
		panic("FAIL: struct empty slice literal field")
	}
}

// Struct initialization
func testStructInitialization() {
	// Empty struct
	c := Composite{}
	if len(c.a) == 0 {
	}

	// Struct with field values
	p := Person{name: "Alice", age: 30}
	if p.name == "Alice" && p.age == 30 {
		fmt.Println("PASS: struct init")
	} else {
		panic("FAIL: struct init")
	}
}

// Nested if statements
func testNestedIf() {
	a := 10
	b := 20

	if a == 10 {
		if b == 20 {
			fmt.Println("PASS: nested if")
		} else {
			panic("FAIL: nested if inner")
		}
	} else {
		panic("FAIL: nested if outer")
	}
}

// Test int32, int64 types (from iceberg)
func testInt32Int64Types() {
	var a int32
	var b int64

	a = 100
	b = 200

	if a == 100 && b == 200 {
		fmt.Println("PASS: int32 int64 values")
	} else {
		panic("FAIL: int32 int64 values")
	}

	// Struct with int32/int64 fields
	record := types.DataRecord{
		ID:          1,
		Size:        1024,
		Count:       10,
		SequenceNum: 999,
	}
	if record.ID == 1 && record.Size == 1024 {
		fmt.Println("PASS: int32 int64 struct fields")
	} else {
		panic("FAIL: int32 int64 struct fields")
	}
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
	if plan.Root == 0 && len(plan.Literals) == 0 {
		fmt.Println("PASS: zero value struct")
	} else {
		panic("FAIL: zero value struct")
	}
}

// Test function returning modified struct
func testReturnModifiedStruct() {
	var plan types.Plan
	plan.Literals = []string{}

	idx := 0
	plan, idx = types.AddLiteralToPlan(plan, "first")
	plan, idx = types.AddLiteralToPlan(plan, "second")

	if idx == 1 && len(plan.Literals) == 2 {
		fmt.Println("PASS: return modified struct")
	} else {
		panic("FAIL: return modified struct")
	}
}

// Test bool field in struct (from iceberg)
func testBoolFieldInStruct() {
	f := types.Field{
		ID:       1,
		Name:     "column1",
		Required: true,
	}
	if f.ID == 1 && f.Name == "column1" && f.Required {
		fmt.Println("PASS: bool field struct required")
	} else {
		panic("FAIL: bool field struct required")
	}

	f2 := types.Field{
		ID:       2,
		Name:     "column2",
		Required: false,
	}
	if f2.ID == 2 && !f2.Required {
		fmt.Println("PASS: bool field struct optional")
	} else {
		panic("FAIL: bool field struct optional")
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

	if entry.Status == 1 && entry.DataFileF.RecordCount == 1000 && entry.DataFileF.Stats.NullCount == 100 {
		fmt.Println("PASS: nested struct fields")
	} else {
		panic("FAIL: nested struct fields")
	}
	if entry.DataFileF.FilePath == "/path/to/file" {
		fmt.Println("PASS: nested struct string field")
	} else {
		panic("FAIL: nested struct string field")
	}
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
	if idx == 1 && len(plan.Literals) == 2 {
		fmt.Println("PASS: multi-package plan")
	} else {
		panic("FAIL: multi-package plan")
	}

	// Use constants from types package
	if types.ExprLiteral == 0 {
		fmt.Println("PASS: multi-package constant")
	} else {
		panic("FAIL: multi-package constant")
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
	if entry.DataFileF.FilePath == "/data/file.parquet" && entry.DataFileF.RecordCount == 500 {
		fmt.Println("PASS: multi-package nested struct")
	} else {
		panic("FAIL: multi-package nested struct")
	}
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
	if intVal == 42 {
		fmt.Println("PASS: type assert int")
	} else {
		panic("FAIL: type assert int")
	}

	a = "world"
	strVal := a.(string)
	if strVal == "world" {
		fmt.Println("PASS: type assert string")
	} else {
		panic("FAIL: type assert string")
	}

	a = true
	boolVal := a.(bool)
	if boolVal {
		fmt.Println("PASS: type assert bool")
	} else {
		panic("FAIL: type assert bool")
	}

	a = 2.71
	floatVal := a.(float64)
	fmt.Println(floatVal)
}

// Helper: sum all elements in a slice and check against expected
func checkSliceSum(s []int, expected int) {
	total := 0
	i := 0
	for i < len(s) {
		total = total + s[i]
		i = i + 1
	}
	if total == expected {
		fmt.Println("PASS: slice sum")
	} else {
		panic("FAIL: slice sum")
	}
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
	if innerTotal == 12 {
		fmt.Println("PASS: nested while")
	} else {
		panic("FAIL: nested while")
	}

	// Condition-only loop nested inside traditional for loop
	sum2 := 0
	for i := 0; i < 3; i++ {
		k := 0
		for k < 2 {
			sum2 = sum2 + 1
			k = k + 1
		}
	}
	if sum2 == 6 {
		fmt.Println("PASS: traditional outer while inner")
	} else {
		panic("FAIL: traditional outer while inner")
	}

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
	if total3 == 8 {
		fmt.Println("PASS: triple nested while")
	} else {
		panic("FAIL: triple nested while")
	}
}

// Inline composite literal as function argument
// Tests that func([]int{1,2,3}) generates correct Rust
func testInlineCompositeLitArg() {
	// Pass inline composite literal to a function
	checkSliceSum([]int{10, 20, 30}, 60)

	// Pass inline composite literal with single element
	checkSliceSum([]int{42}, 42)

	// Pass inline composite literal with many elements
	checkSliceSum([]int{1, 2, 3, 4, 5}, 15)
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
	if getMapLen(m) == 2 && m["hello"] == 1 && m["world"] == 2 {
		fmt.Println("PASS: map as parameter")
	} else {
		panic("FAIL: map as parameter")
	}
}

// Test map operations: make, get, set, delete, len with string/int/bool keys
func testMapOperations() {
	// String keys
	m := make(map[string]int)
	m["hello"] = 1
	m["world"] = 2
	if m["hello"] == 1 && m["world"] == 2 && len(m) == 2 {
		fmt.Println("PASS: map string keys")
	} else {
		panic("FAIL: map string keys")
	}
	delete(m, "hello")
	if len(m) == 1 {
		fmt.Println("PASS: map delete")
	} else {
		panic("FAIL: map delete")
	}

	// Int keys
	m2 := make(map[int]string)
	m2[42] = "answer"
	m2[7] = "lucky"
	if m2[42] == "answer" && len(m2) == 2 {
		fmt.Println("PASS: map int keys")
	} else {
		panic("FAIL: map int keys")
	}

	// Bool keys
	m3 := make(map[bool]int)
	m3[true] = 1
	m3[false] = 0
	if m3[true] == 1 && m3[false] == 0 {
		fmt.Println("PASS: map bool keys")
	} else {
		panic("FAIL: map bool keys")
	}
}

func testNilMap() {
	var m map[string]int
	if len(m) == 0 {
		fmt.Println("PASS: nil map len")
	} else {
		panic("FAIL: nil map len")
	}
	m = make(map[string]int)
	m["x"] = 10
	if m["x"] == 10 && len(m) == 1 {
		fmt.Println("PASS: nil map after make")
	} else {
		panic("FAIL: nil map after make")
	}
}

func testMapKeyTypes() {
	m1 := make(map[int64]string)
	m1[100] = "hundred"
	if m1[100] == "hundred" && len(m1) == 1 {
		fmt.Println("PASS: map int64 key")
	} else {
		panic("FAIL: map int64 key")
	}

	m2 := make(map[float64]int)
	m2[3.14] = 314
	if m2[3.14] == 314 && len(m2) == 1 {
		fmt.Println("PASS: map float64 key")
	} else {
		panic("FAIL: map float64 key")
	}

	m3 := make(map[int32]string)
	m3[42] = "answer"
	if m3[42] == "answer" && len(m3) == 1 {
		fmt.Println("PASS: map int32 key")
	} else {
		panic("FAIL: map int32 key")
	}
}

type MapTestStruct struct {
	Name  string
	Value int
}

func testMapStructValue() {
	m := make(map[string]MapTestStruct)
	m["first"] = MapTestStruct{Name: "hello", Value: 42}
	s := m["first"]
	if s.Name == "hello" && s.Value == 42 && len(m) == 1 {
		fmt.Println("PASS: map struct value")
	} else {
		panic("FAIL: map struct value")
	}
}

func testMapCommaOk() {
	fmt.Println("=== Map Comma-Ok ===")
	m := make(map[string]int)
	m["hello"] = 42

	val, ok := m["hello"]
	if val == 42 && ok {
		fmt.Println("PASS: map comma-ok found")
	} else {
		panic("FAIL: map comma-ok found")
	}

	val2, ok2 := m["missing"]
	if val2 == 0 && !ok2 {
		fmt.Println("PASS: map comma-ok missing")
	} else {
		panic("FAIL: map comma-ok missing")
	}

	if len(m) == 1 {
		fmt.Println("PASS: map comma-ok len")
	} else {
		panic("FAIL: map comma-ok len")
	}
}

func testTypeAssertCommaOk() {
	fmt.Println("=== Type Assert Comma-Ok ===")
	var x interface{}
	x = 42
	val, ok := x.(int)
	if val == 42 && ok {
		fmt.Println("PASS: type assert comma-ok int")
	} else {
		panic("FAIL: type assert comma-ok int")
	}

	val2, ok2 := x.(string)
	fmt.Println(val2)
	fmt.Println(ok2)
}

func testIfInitCommaOk() {
	fmt.Println("=== If-Init Comma-Ok ===")
	m := make(map[string]int)
	m["hello"] = 42

	if val, ok := m["hello"]; ok {
		if val == 42 {
			fmt.Println("PASS: if-init map found")
		} else {
			panic("FAIL: if-init map found wrong val")
		}
	} else {
		panic("FAIL: if-init map found not entered")
	}
	if val, ok := m["missing"]; ok {
		fmt.Println(val)
		panic("FAIL: if-init map missing entered")
	} else {
		fmt.Println("PASS: if-init map missing")
	}

	var x interface{}
	x = 10
	if val, ok := x.(int); ok {
		if val == 10 {
			fmt.Println("PASS: if-init type assert")
		} else {
			panic("FAIL: if-init type assert wrong val")
		}
	} else {
		panic("FAIL: if-init type assert not entered")
	}
}

type MapFieldStruct struct {
	Settings map[string]int
}

func testMapStructField() {
	fmt.Println("=== Map Struct Field ===")
	s := MapFieldStruct{}
	s.Settings = make(map[string]int)
	s.Settings["timeout"] = 30
	s.Settings["retries"] = 3

	// get + len
	if s.Settings["timeout"] == 30 && len(s.Settings) == 2 {
		fmt.Println("PASS: map struct field get/len")
	} else {
		panic("FAIL: map struct field get/len")
	}

	// delete
	delete(s.Settings, "retries")
	if len(s.Settings) == 1 {
		fmt.Println("PASS: map struct field delete")
	} else {
		panic("FAIL: map struct field delete")
	}

	// comma-ok
	val, ok := s.Settings["timeout"]
	if val == 30 && ok {
		fmt.Println("PASS: map struct field comma-ok")
	} else {
		panic("FAIL: map struct field comma-ok")
	}

	val2, ok2 := s.Settings["missing"]
	if val2 == 0 && !ok2 {
		fmt.Println("PASS: map struct field comma-ok missing")
	} else {
		panic("FAIL: map struct field comma-ok missing")
	}
}

// Helper function that returns a map
func createStringIntMap() map[string]int {
	m := make(map[string]int)
	m["one"] = 1
	m["two"] = 2
	m["three"] = 3
	return m
}

// Helper function that returns an empty map
func createEmptyMap() map[string]int {
	return make(map[string]int)
}

// Helper function that modifies and returns a map
func addToMap(m map[string]int, key string, value int) map[string]int {
	m[key] = value
	return m
}

func testNestedSlices() {
	fmt.Println("=== Nested Slices ===")

	// 2D slice: [][]int
	var m [][]int
	m = make([][]int, 2)
	m[0] = make([]int, 3)
	m[1] = make([]int, 2)
	m[0][0] = 1
	m[0][1] = 2
	m[0][2] = 3
	m[1][0] = 4
	m[1][1] = 5

	if len(m) == 2 && len(m[0]) == 3 && len(m[1]) == 2 {
		fmt.Println("PASS: nested slice dimensions")
	} else {
		panic("FAIL: nested slice dimensions")
	}

	if m[0][0] == 1 && m[0][1] == 2 && m[0][2] == 3 && m[1][0] == 4 && m[1][1] == 5 {
		fmt.Println("PASS: nested slice values")
	} else {
		panic("FAIL: nested slice values")
	}

	// Sum all elements
	sum := 0
	for i := 0; i < len(m); i++ {
		for j := 0; j < len(m[i]); j++ {
			sum = sum + m[i][j]
		}
	}
	if sum == 15 {
		fmt.Println("PASS: nested slice sum")
	} else {
		panic("FAIL: nested slice sum")
	}
}

func testMapReturnValue() {
	fmt.Println("=== Map Return Value ===")

	// Test 1: Basic map return
	m1 := createStringIntMap()
	if m1["one"] == 1 && m1["two"] == 2 && m1["three"] == 3 && len(m1) == 3 {
		fmt.Println("PASS: map return value basic")
	} else {
		panic("FAIL: map return value basic")
	}

	// Test 2: Empty map return
	m2 := createEmptyMap()
	if len(m2) == 0 {
		fmt.Println("PASS: map return value empty")
	} else {
		panic("FAIL: map return value empty")
	}

	// Test 3: Map passed and returned
	m3 := make(map[string]int)
	m3["start"] = 100
	m3 = addToMap(m3, "added", 200)
	if m3["start"] == 100 && m3["added"] == 200 && len(m3) == 2 {
		fmt.Println("PASS: map return value modified")
	} else {
		panic("FAIL: map return value modified")
	}

	// Test 4: Chained map operations with return
	m4 := addToMap(createEmptyMap(), "chained", 42)
	if m4["chained"] == 42 && len(m4) == 1 {
		fmt.Println("PASS: map return value chained")
	} else {
		panic("FAIL: map return value chained")
	}
}

func testNestedMaps() {
	fmt.Println("=== Nested Maps ===")

	// Basic nested map assignment: map[string]map[string]int
	m := make(map[string]map[string]int)
	m["outer"] = make(map[string]int)
	m["outer"]["inner"] = 42
	m["outer"]["second"] = 100

	// Test nested map read: m["outer"]["inner"]
	val := m["outer"]["inner"]
	if val == 42 {
		fmt.Println("PASS: nested map read")
	} else {
		panic("FAIL: nested map read")
	}

	// Test nested map read with multiple keys
	val2 := m["outer"]["second"]
	if val2 == 100 {
		fmt.Println("PASS: nested map read second key")
	} else {
		panic("FAIL: nested map read second key")
	}
}

func testMixedNestedComposites() {
	fmt.Println("=== Mixed Nested Composites ===")

	// Test 1: []map[string]int - slice of maps
	var sliceOfMaps []map[string]int
	sliceOfMaps = make([]map[string]int, 2)
	sliceOfMaps[0] = make(map[string]int)
	sliceOfMaps[1] = make(map[string]int)
	sliceOfMaps[0]["a"] = 10
	sliceOfMaps[0]["b"] = 20
	sliceOfMaps[1]["c"] = 30

	// Read from slice of maps
	val1 := sliceOfMaps[0]["a"]
	val2 := sliceOfMaps[0]["b"]
	val3 := sliceOfMaps[1]["c"]
	if val1 == 10 && val2 == 20 && val3 == 30 {
		fmt.Println("PASS: slice of maps read")
	} else {
		panic("FAIL: slice of maps read")
	}

	// Test 2: map[string][]int - map of slices
	var mapOfSlices map[string][]int
	mapOfSlices = make(map[string][]int)
	mapOfSlices["first"] = make([]int, 3)
	mapOfSlices["second"] = make([]int, 2)
	mapOfSlices["first"][0] = 100
	mapOfSlices["first"][1] = 200
	mapOfSlices["first"][2] = 300
	mapOfSlices["second"][0] = 400
	mapOfSlices["second"][1] = 500

	// Read from map of slices
	v1 := mapOfSlices["first"][0]
	v2 := mapOfSlices["first"][2]
	v3 := mapOfSlices["second"][1]
	if v1 == 100 && v2 == 300 && v3 == 500 {
		fmt.Println("PASS: map of slices read")
	} else {
		panic("FAIL: map of slices read")
	}

	// Test 3: [][]map[string]int - nested slice of maps
	var nestedSliceOfMaps [][]map[string]int
	nestedSliceOfMaps = make([][]map[string]int, 2)
	nestedSliceOfMaps[0] = make([]map[string]int, 2)
	nestedSliceOfMaps[1] = make([]map[string]int, 1)
	nestedSliceOfMaps[0][0] = make(map[string]int)
	nestedSliceOfMaps[0][1] = make(map[string]int)
	nestedSliceOfMaps[1][0] = make(map[string]int)
	nestedSliceOfMaps[0][0]["x"] = 1
	nestedSliceOfMaps[0][1]["y"] = 2
	nestedSliceOfMaps[1][0]["z"] = 3

	// Read from nested slice of maps
	r1 := nestedSliceOfMaps[0][0]["x"]
	r2 := nestedSliceOfMaps[0][1]["y"]
	r3 := nestedSliceOfMaps[1][0]["z"]
	if r1 == 1 && r2 == 2 && r3 == 3 {
		fmt.Println("PASS: nested slice of maps read")
	} else {
		panic("FAIL: nested slice of maps read")
	}

	// Test 4: map[string]map[string][]int - nested maps with slice values
	var nestedMapsWithSlice map[string]map[string][]int
	nestedMapsWithSlice = make(map[string]map[string][]int)
	nestedMapsWithSlice["outer"] = make(map[string][]int)
	nestedMapsWithSlice["outer"]["inner"] = make([]int, 3)
	nestedMapsWithSlice["outer"]["inner"][0] = 111
	nestedMapsWithSlice["outer"]["inner"][1] = 222
	nestedMapsWithSlice["outer"]["inner"][2] = 333

	// Read from nested maps with slice
	s1 := nestedMapsWithSlice["outer"]["inner"][0]
	s2 := nestedMapsWithSlice["outer"]["inner"][1]
	s3 := nestedMapsWithSlice["outer"]["inner"][2]
	if s1 == 111 && s2 == 222 && s3 == 333 {
		fmt.Println("PASS: nested maps with slice read")
	} else {
		panic("FAIL: nested maps with slice read")
	}

	// Test 5: map[string][][]int - map of nested slices
	var mapOfNestedSlices map[string][][]int
	mapOfNestedSlices = make(map[string][][]int)
	mapOfNestedSlices["matrix"] = make([][]int, 2)
	mapOfNestedSlices["matrix"][0] = make([]int, 2)
	mapOfNestedSlices["matrix"][1] = make([]int, 2)
	mapOfNestedSlices["matrix"][0][0] = 11
	mapOfNestedSlices["matrix"][0][1] = 12
	mapOfNestedSlices["matrix"][1][0] = 21
	mapOfNestedSlices["matrix"][1][1] = 22

	// Read from map of nested slices
	m1 := mapOfNestedSlices["matrix"][0][0]
	m2 := mapOfNestedSlices["matrix"][0][1]
	m3 := mapOfNestedSlices["matrix"][1][0]
	m4 := mapOfNestedSlices["matrix"][1][1]
	if m1 == 11 && m2 == 12 && m3 == 21 && m4 == 22 {
		fmt.Println("PASS: map of nested slices read")
	} else {
		panic("FAIL: map of nested slices read")
	}

	// Test 6: [][][]map[string]int - triple nested slice of maps
	var tripleNestedSliceOfMaps [][][]map[string]int
	tripleNestedSliceOfMaps = make([][][]map[string]int, 1)
	tripleNestedSliceOfMaps[0] = make([][]map[string]int, 1)
	tripleNestedSliceOfMaps[0][0] = make([]map[string]int, 1)
	tripleNestedSliceOfMaps[0][0][0] = make(map[string]int)
	tripleNestedSliceOfMaps[0][0][0]["deep"] = 999

	// Read from triple nested slice of maps
	deep := tripleNestedSliceOfMaps[0][0][0]["deep"]
	if deep == 999 {
		fmt.Println("PASS: triple nested slice of maps read")
	} else {
		panic("FAIL: triple nested slice of maps read")
	}

	// Test 7: map[int][]map[string]int - map with int key, slice of maps value
	var intKeySliceOfMaps map[int][]map[string]int
	intKeySliceOfMaps = make(map[int][]map[string]int)
	intKeySliceOfMaps[1] = make([]map[string]int, 2)
	intKeySliceOfMaps[1][0] = make(map[string]int)
	intKeySliceOfMaps[1][1] = make(map[string]int)
	intKeySliceOfMaps[1][0]["foo"] = 777
	intKeySliceOfMaps[1][1]["bar"] = 888

	// Read from map with int key
	ik1 := intKeySliceOfMaps[1][0]["foo"]
	ik2 := intKeySliceOfMaps[1][1]["bar"]
	if ik1 == 777 && ik2 == 888 {
		fmt.Println("PASS: int key map of slice of maps read")
	} else {
		panic("FAIL: int key map of slice of maps read")
	}
}

// testBuiltinFunctions tests all supported Go builtin functions
// Supported: len, append, make, delete, panic
// NOT supported: cap, copy, new, close, recover, complex, real, imag
func testBuiltinFunctions() {
	fmt.Println("Testing builtin functions...")

	// Test len() on slices
	slice := []int{1, 2, 3, 4, 5}
	if len(slice) == 5 {
		fmt.Println("PASS: len() on slice")
	} else {
		panic("FAIL: len() on slice")
	}

	// Test len() on strings
	str := "hello"
	if len(str) == 5 {
		fmt.Println("PASS: len() on string")
	} else {
		panic("FAIL: len() on string")
	}

	// Test len() on maps
	m := make(map[string]int)
	m["a"] = 1
	m["b"] = 2
	if len(m) == 2 {
		fmt.Println("PASS: len() on map")
	} else {
		panic("FAIL: len() on map")
	}

	// Test append() - basic
	s := []int{1, 2}
	s = append(s, 3)
	if len(s) == 3 && s[2] == 3 {
		fmt.Println("PASS: append() basic")
	} else {
		panic("FAIL: append() basic")
	}

	// Test append() - sequential appends
	s2 := []int{1}
	s2 = append(s2, 2)
	s2 = append(s2, 3)
	s2 = append(s2, 4)
	if len(s2) == 4 && s2[3] == 4 {
		fmt.Println("PASS: append() sequential")
	} else {
		panic("FAIL: append() sequential")
	}

	// Test make() - slice with length (just verify length, don't check zero value)
	sliceWithLen := make([]int, 5)
	if len(sliceWithLen) == 5 {
		// Note: Java initializes with null, other backends with 0
		// Just verify the length is correct
		fmt.Println("PASS: make() slice with length")
	} else {
		panic("FAIL: make() slice with length")
	}

	// Test make() - map
	mapMade := make(map[string]bool)
	mapMade["test"] = true
	if mapMade["test"] == true {
		fmt.Println("PASS: make() map")
	} else {
		panic("FAIL: make() map")
	}

	// Test delete() on map
	m2 := make(map[string]int)
	m2["x"] = 10
	m2["y"] = 20
	delete(m2, "x")
	if len(m2) == 1 {
		fmt.Println("PASS: delete() on map")
	} else {
		panic("FAIL: delete() on map")
	}

	// Test delete() on non-existent key (should not panic)
	delete(m2, "nonexistent")
	fmt.Println("PASS: delete() non-existent key")

	// Test len() on empty slice
	emptySlice := []string{}
	if len(emptySlice) == 0 {
		fmt.Println("PASS: len() on empty slice")
	} else {
		panic("FAIL: len() on empty slice")
	}

	// Test len() on nil slice (declared but not initialized)
	var nilSlice []int
	if len(nilSlice) == 0 {
		fmt.Println("PASS: len() on nil slice")
	} else {
		panic("FAIL: len() on nil slice")
	}

	// Test append() on nil slice
	var nilSlice2 []int
	nilSlice2 = append(nilSlice2, 42)
	if len(nilSlice2) == 1 && nilSlice2[0] == 42 {
		fmt.Println("PASS: append() on nil slice")
	} else {
		panic("FAIL: append() on nil slice")
	}

	// Test len() on empty map
	emptyMap := make(map[int]int)
	if len(emptyMap) == 0 {
		fmt.Println("PASS: len() on empty map")
	} else {
		panic("FAIL: len() on empty map")
	}

	// Test make() slice with zero length
	zeroSlice := make([]int, 0)
	if len(zeroSlice) == 0 {
		fmt.Println("PASS: make() slice with zero length")
	} else {
		panic("FAIL: make() slice with zero length")
	}

	// Test chained append
	var chain []int
	chain = append(chain, 1)
	chain = append(chain, 2)
	chain = append(chain, 3)
	if len(chain) == 3 && chain[0] == 1 && chain[1] == 2 && chain[2] == 3 {
		fmt.Println("PASS: chained append()")
	} else {
		panic("FAIL: chained append()")
	}

	fmt.Println("All builtin function tests passed!")
}

// Helper function for testing multi-return
func getMultipleValues() (int, string, bool) {
	return 42, "hello", true
}

// Helper function returning two ints
func getTwoInts() (int, int) {
	return 10, 20
}

// testMultiVariablePatterns tests the supported multi-variable assignment patterns
// Supported: map comma-ok, type assertion comma-ok, function multi-return
// NOT supported: a, b := 1, 2 (literal tuple unpacking)
func testMultiVariablePatterns() {
	fmt.Println("Testing multi-variable patterns...")

	// Pattern 1: Function returning multiple values
	a, b, c := getMultipleValues()
	if a == 42 && b == "hello" && c == true {
		fmt.Println("PASS: function multi-return (3 values)")
	} else {
		panic("FAIL: function multi-return (3 values)")
	}

	// Pattern 2: Function returning two values
	x, y := getTwoInts()
	if x == 10 && y == 20 {
		fmt.Println("PASS: function multi-return (2 values)")
	} else {
		panic("FAIL: function multi-return (2 values)")
	}

	// Pattern 3: Map comma-ok idiom
	m := make(map[string]int)
	m["key"] = 100
	val, ok := m["key"]
	if val == 100 && ok {
		fmt.Println("PASS: map comma-ok (found)")
	} else {
		panic("FAIL: map comma-ok (found)")
	}

	val2, ok2 := m["missing"]
	if val2 == 0 && !ok2 {
		fmt.Println("PASS: map comma-ok (missing)")
	} else {
		panic("FAIL: map comma-ok (missing)")
	}

	// Pattern 4: Type assertion comma-ok
	var iface interface{}
	iface = 999
	intVal, isInt := iface.(int)
	if intVal == 999 && isInt {
		fmt.Println("PASS: type assertion comma-ok (success)")
	} else {
		panic("FAIL: type assertion comma-ok (success)")
	}

	// Type assertion that should fail - just print result
	// (backends may differ on exact behavior for failed assertions)
	strVal, isStr := iface.(string)
	fmt.Println(strVal)
	fmt.Println(isStr)

	// Pattern 5: Reassignment with function multi-return
	x, y = getTwoInts()
	x = x + 1
	y = y + 1
	if x == 11 && y == 21 {
		fmt.Println("PASS: reassignment with multi-return")
	} else {
		panic("FAIL: reassignment with multi-return")
	}

	fmt.Println("All multi-variable pattern tests passed!")
}

// Point is a simple struct with primitive fields only - can be used as map key
type Point struct {
	X int
	Y int
}

// testStructMapKeys tests using structs as map keys
func testStructMapKeys() {
	fmt.Println("Testing struct map keys...")

	// Create a map with struct keys
	m := make(map[Point]string)

	// Add some entries
	p1 := Point{X: 1, Y: 2}
	p2 := Point{X: 3, Y: 4}
	p3 := Point{X: 1, Y: 2} // Same as p1

	m[p1] = "first"
	m[p2] = "second"

	// Test retrieval
	if m[p1] == "first" {
		fmt.Println("PASS: struct key retrieval")
	} else {
		panic("FAIL: struct key retrieval")
	}

	// Test that equal structs map to same key
	if m[p3] == "first" {
		fmt.Println("PASS: equal struct keys")
	} else {
		panic("FAIL: equal struct keys")
	}

	// Test map length
	if len(m) == 2 {
		fmt.Println("PASS: struct key map length")
	} else {
		panic("FAIL: struct key map length")
	}

	// Test update
	m[p1] = "updated"
	if m[p3] == "updated" {
		fmt.Println("PASS: struct key update")
	} else {
		panic("FAIL: struct key update")
	}

	// Test delete
	delete(m, p1)
	if len(m) == 1 {
		fmt.Println("PASS: struct key delete")
	} else {
		panic("FAIL: struct key delete")
	}

	fmt.Println("All struct map key tests passed!")
}

// Inner is a simple struct used as a field in Outer
type Inner struct {
	A int
	B int
}

// Outer contains an Inner struct - tests nested struct map keys
type Outer struct {
	I Inner
	C int
}

// testNestedStructMapKeys tests using structs with nested struct fields as map keys
func testNestedStructMapKeys() {
	fmt.Println("Testing nested struct map keys...")

	m := make(map[Outer]string)

	o1 := Outer{I: Inner{A: 1, B: 2}, C: 10}
	o2 := Outer{I: Inner{A: 3, B: 4}, C: 20}
	o3 := Outer{I: Inner{A: 1, B: 2}, C: 10} // Same as o1

	m[o1] = "first"
	m[o2] = "second"

	// Test retrieval
	if m[o1] == "first" {
		fmt.Println("PASS: nested struct key retrieval")
	} else {
		panic("FAIL: nested struct key retrieval")
	}

	// Test that equal nested structs map to same key
	if m[o3] == "first" {
		fmt.Println("PASS: equal nested struct keys")
	} else {
		panic("FAIL: equal nested struct keys")
	}

	// Test map length
	if len(m) == 2 {
		fmt.Println("PASS: nested struct key map length")
	} else {
		panic("FAIL: nested struct key map length")
	}

	// Test update via equal key
	m[o3] = "updated"
	if m[o1] == "updated" {
		fmt.Println("PASS: nested struct key update")
	} else {
		panic("FAIL: nested struct key update")
	}

	// Test delete
	delete(m, o1)
	if len(m) == 1 {
		fmt.Println("PASS: nested struct key delete")
	} else {
		panic("FAIL: nested struct key delete")
	}

	fmt.Println("All nested struct map key tests passed!")
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
	testNilSliceAppend()
	testStructNilSliceField()
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
	testMapStructField()
	testMapReturnValue()
	testNestedSlices()
	testNestedMaps()
	testMixedNestedComposites()
	testBuiltinFunctions()
	testMultiVariablePatterns()
	testStructMapKeys()
	testNestedStructMapKeys()

	fmt.Println("=== Done ===")
}
