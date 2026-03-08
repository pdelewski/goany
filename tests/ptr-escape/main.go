package main

import "fmt"

// Struct types for testing
type Person struct {
	name string
	age  int
}

type Rect struct {
	w int
	h int
}

// --- Helper functions ---

func modifyInt(p *int) {
	*p = 99
}

func modifyString(p *string) {
	*p = "modified"
}

func addToInt(p *int, n int) {
	*p += n
}

func readInt(p *int) int {
	return *p
}

func swapIntPtrs(a *int, b *int) {
	tmp := *a
	*a = *b
	*b = tmp
}

func returnFieldPtr(s Person) *int {
	return &s.age
}

func returnIndexPtr(arr []int, i int) *int {
	return &arr[i]
}

// --- Test cases: &s.field downward escape (pass to function) ---

func testFieldEscapeModifyInt() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	modifyInt(p)
	if s.age == 99 {
		fmt.Println("PASS: field escape modify int")
	} else {
		fmt.Println("FAIL: field escape modify int")
	}
}

func testFieldEscapeModifyString() {
	s := Person{name: "Alice", age: 30}
	p := &s.name
	modifyString(p)
	if s.name == "modified" {
		fmt.Println("PASS: field escape modify string")
	} else {
		fmt.Println("FAIL: field escape modify string")
	}
}

func testFieldEscapeAddToInt() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	addToInt(p, 10)
	if s.age == 40 {
		fmt.Println("PASS: field escape add to int")
	} else {
		fmt.Println("FAIL: field escape add to int")
	}
}

func testFieldEscapeReadOnly() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	val := readInt(p)
	if val == 30 {
		fmt.Println("PASS: field escape read only")
	} else {
		fmt.Println("FAIL: field escape read only")
	}
}

func testFieldEscapeMultipleCalls() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	addToInt(p, 5)
	addToInt(p, 3)
	if s.age == 38 {
		fmt.Println("PASS: field escape multiple calls")
	} else {
		fmt.Println("FAIL: field escape multiple calls")
	}
}

func testFieldEscapeTwoFields() {
	s := Person{name: "Alice", age: 30}
	p1 := &s.age
	p2 := &s.name
	modifyInt(p1)
	modifyString(p2)
	if s.age == 99 && s.name == "modified" {
		fmt.Println("PASS: field escape two fields")
	} else {
		fmt.Println("FAIL: field escape two fields")
	}
}

// --- Test cases: &arr[i] downward escape (pass to function) ---

func testIndexEscapeModifyInt() {
	arr := []int{10, 20, 30}
	p := &arr[1]
	modifyInt(p)
	if arr[1] == 99 {
		fmt.Println("PASS: index escape modify int")
	} else {
		fmt.Println("FAIL: index escape modify int")
	}
}

func testIndexEscapeVariableIndex() {
	arr := []int{10, 20, 30}
	idx := 2
	p := &arr[idx]
	modifyInt(p)
	if arr[2] == 99 {
		fmt.Println("PASS: index escape variable index")
	} else {
		fmt.Println("FAIL: index escape variable index")
	}
}

func testIndexEscapeAddToInt() {
	arr := []int{10, 20, 30}
	p := &arr[0]
	addToInt(p, 5)
	if arr[0] == 15 {
		fmt.Println("PASS: index escape add to int")
	} else {
		fmt.Println("FAIL: index escape add to int")
	}
}

func testIndexEscapeMultipleCalls() {
	arr := []int{10, 20, 30}
	p := &arr[1]
	addToInt(p, 1)
	addToInt(p, 2)
	if arr[1] == 23 {
		fmt.Println("PASS: index escape multiple calls")
	} else {
		fmt.Println("FAIL: index escape multiple calls")
	}
}

func testIndexEscapeSwap() {
	arr := []int{10, 20, 30}
	p1 := &arr[0]
	p2 := &arr[2]
	swapIntPtrs(p1, p2)
	if arr[0] == 30 && arr[2] == 10 {
		fmt.Println("PASS: index escape swap")
	} else {
		fmt.Println("FAIL: index escape swap")
	}
}

// --- Test cases: upward escape (return from function) ---

func testFieldReturnPtr() {
	s := Person{name: "Alice", age: 30}
	p := returnFieldPtr(s)
	val := *p
	if val == 30 {
		fmt.Println("PASS: field return ptr")
	} else {
		fmt.Println("FAIL: field return ptr")
	}
}

func testIndexReturnPtr() {
	arr := []int{10, 20, 30}
	p := returnIndexPtr(arr, 1)
	val := *p
	if val == 20 {
		fmt.Println("PASS: index return ptr")
	} else {
		fmt.Println("FAIL: index return ptr")
	}
}

// --- Test cases: local mutation (no escape, pool-based) ---

func testFieldLocalMutate() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	*p = 25
	if s.age == 25 {
		fmt.Println("PASS: field local mutate")
	} else {
		fmt.Println("FAIL: field local mutate")
	}
}

func testFieldLocalRead() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	val := *p
	if val == 30 {
		fmt.Println("PASS: field local read")
	} else {
		fmt.Println("FAIL: field local read")
	}
}

func testIndexLocalMutate() {
	arr := []int{10, 20, 30}
	p := &arr[1]
	*p = 99
	if arr[1] == 99 {
		fmt.Println("PASS: index local mutate")
	} else {
		fmt.Println("FAIL: index local mutate")
	}
}

func testIndexLocalRead() {
	arr := []int{10, 20, 30}
	p := &arr[1]
	val := *p
	if val == 20 {
		fmt.Println("PASS: index local read")
	} else {
		fmt.Println("FAIL: index local read")
	}
}

// --- Test cases: multiple pointer types in same function ---

func testMultiplePoolTypes() {
	s := Person{name: "Alice", age: 30}
	p1 := &s.age
	p2 := &s.name
	modifyInt(p1)
	modifyString(p2)
	if s.age == 99 && s.name == "modified" {
		fmt.Println("PASS: multiple pool types")
	} else {
		fmt.Println("FAIL: multiple pool types")
	}
}

func testMixedFieldAndIndex() {
	s := Person{name: "Alice", age: 30}
	arr := []int{10, 20, 30}
	p1 := &s.age
	p2 := &arr[0]
	modifyInt(p1)
	modifyInt(p2)
	if s.age == 99 && arr[0] == 99 {
		fmt.Println("PASS: mixed field and index")
	} else {
		fmt.Println("FAIL: mixed field and index")
	}
}

// --- Test cases: interaction with existing pointer patterns ---

func testFieldEscapeWithSimplePtr() {
	x := 5
	s := Person{name: "Alice", age: 30}
	p1 := &x
	p2 := &s.age
	modifyInt(p1)
	modifyInt(p2)
	if x == 99 && s.age == 99 {
		fmt.Println("PASS: field escape with simple ptr")
	} else {
		fmt.Println("FAIL: field escape with simple ptr")
	}
}

func testIndexEscapeWithSimplePtr() {
	x := 5
	arr := []int{10, 20, 30}
	p1 := &x
	p2 := &arr[0]
	modifyInt(p1)
	modifyInt(p2)
	if x == 99 && arr[0] == 99 {
		fmt.Println("PASS: index escape with simple ptr")
	} else {
		fmt.Println("FAIL: index escape with simple ptr")
	}
}

// --- Test cases: struct with multiple int fields ---

func testTwoFieldsSameType() {
	r := Rect{w: 10, h: 20}
	p1 := &r.w
	p2 := &r.h
	modifyInt(p1)
	addToInt(p2, 5)
	if r.w == 99 && r.h == 25 {
		fmt.Println("PASS: two fields same type")
	} else {
		fmt.Println("FAIL: two fields same type")
	}
}

func main() {
	// &s.field downward escape
	testFieldEscapeModifyInt()
	testFieldEscapeModifyString()
	testFieldEscapeAddToInt()
	testFieldEscapeReadOnly()
	testFieldEscapeMultipleCalls()
	testFieldEscapeTwoFields()

	// &arr[i] downward escape
	testIndexEscapeModifyInt()
	testIndexEscapeVariableIndex()
	testIndexEscapeAddToInt()
	testIndexEscapeMultipleCalls()
	testIndexEscapeSwap()

	// Upward escape
	testFieldReturnPtr()
	testIndexReturnPtr()

	// Local mutation
	testFieldLocalMutate()
	testFieldLocalRead()
	testIndexLocalMutate()
	testIndexLocalRead()

	// Multiple pool types
	testMultiplePoolTypes()
	testMixedFieldAndIndex()

	// Interaction with existing pointer patterns
	testFieldEscapeWithSimplePtr()
	testIndexEscapeWithSimplePtr()

	// Struct with multiple fields
	testTwoFieldsSameType()
}
