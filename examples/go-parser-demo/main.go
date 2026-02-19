package main

import (
	"fmt"
	"libs/goparser"
)

func main() {
	// Test 1: Package declaration
	code1 := `package main`

	fmt.Println("=== Test 1: Package Declaration ===")
	ast1 := goparser.Parse(code1)
	fmt.Println(goparser.PrintAST(ast1))

	// Test 2: Multiple variable declarations
	code2 := `package main

var a int = 1
var b int = 2
var c int = 3`

	fmt.Println("=== Test 2: Multiple Variable Declarations ===")
	ast2 := goparser.Parse(code2)
	fmt.Println(goparser.PrintAST(ast2))

	// Test 3: Function declaration
	code3 := `package main

func add(a int, b int) int {
	return a + b
}`

	fmt.Println("=== Test 3: Function Declaration ===")
	ast3 := goparser.Parse(code3)
	fmt.Println(goparser.PrintAST(ast3))

	// Test 4: Variable declarations
	code4 := `package main

var x int
var y int = 10`

	fmt.Println("=== Test 4: Variable Declarations ===")
	ast4 := goparser.Parse(code4)
	fmt.Println(goparser.PrintAST(ast4))

	// Test 5: Short variable declaration
	code5 := `package main

func main() {
	x := 10
	y := x + 5
}`

	fmt.Println("=== Test 5: Short Variable Declaration ===")
	ast5 := goparser.Parse(code5)
	fmt.Println(goparser.PrintAST(ast5))

	// Test 6: If statement
	code6 := `package main

func main() {
	if x > 0 {
		y = x
	} else {
		y = 0
	}
}`

	fmt.Println("=== Test 6: If Statement ===")
	ast6 := goparser.Parse(code6)
	fmt.Println(goparser.PrintAST(ast6))

	// Test 7: If with init statement
	code7 := `package main

func main() {
	if err := doSomething(); err != nil {
		return
	}
}`

	fmt.Println("=== Test 7: If with Init Statement ===")
	ast7 := goparser.Parse(code7)
	fmt.Println(goparser.PrintAST(ast7))

	// Test 8: If-else if-else chain
	code8 := `package main

func main() {
	if x > 0 {
		y = 1
	} else if x < 0 {
		y = -1
	} else {
		y = 0
	}
}`

	fmt.Println("=== Test 8: If-Else If-Else Chain ===")
	ast8 := goparser.Parse(code8)
	fmt.Println(goparser.PrintAST(ast8))

	// Test 9: For loop (3-clause)
	code9 := `package main

func main() {
	for i := 0; i < 10; i++ {
		sum += i
	}
}`

	fmt.Println("=== Test 9: For Loop (3-clause) ===")
	ast9 := goparser.Parse(code9)
	fmt.Println(goparser.PrintAST(ast9))

	// Test 10: For loop (condition only)
	code10 := `package main

func main() {
	for n > 0 {
		n--
	}
}`

	fmt.Println("=== Test 10: For Loop (condition) ===")
	ast10 := goparser.Parse(code10)
	fmt.Println(goparser.PrintAST(ast10))

	// Test 11: For range
	code11 := `package main

func main() {
	for i, v := range items {
		fmt.Println(i, v)
	}
}`

	fmt.Println("=== Test 11: For Range ===")
	ast11 := goparser.Parse(code11)
	fmt.Println(goparser.PrintAST(ast11))

	// Test 12: Switch statement
	code12 := `package main

func main() {
	switch x {
	case 1:
		y = 10
	case 2:
		y = 20
	default:
		y = 0
	}
}`

	fmt.Println("=== Test 12: Switch Statement ===")
	ast12 := goparser.Parse(code12)
	fmt.Println(goparser.PrintAST(ast12))

	// Test 13: Struct type declaration
	code13 := `package main

type Point struct {
	X int
	Y int
}`

	fmt.Println("=== Test 13: Struct Declaration ===")
	ast13 := goparser.Parse(code13)
	fmt.Println(goparser.PrintAST(ast13))

	// Test 14: Interface type declaration
	code14 := `package main

type Reader interface {
	Read(p []byte) (int, error)
}`

	fmt.Println("=== Test 14: Interface Declaration ===")
	ast14 := goparser.Parse(code14)
	fmt.Println(goparser.PrintAST(ast14))

	// Test 15: Method declaration
	code15 := `package main

func (p Point) Distance() float64 {
	return 0.0
}`

	fmt.Println("=== Test 15: Method Declaration ===")
	ast15 := goparser.Parse(code15)
	fmt.Println(goparser.PrintAST(ast15))

	// Test 16: Pointer receiver method
	code16 := `package main

func (p *Point) SetX(x int) {
	p.X = x
}`

	fmt.Println("=== Test 16: Pointer Receiver Method ===")
	ast16 := goparser.Parse(code16)
	fmt.Println(goparser.PrintAST(ast16))

	// Test 17: Const declarations
	code17 := `package main

const MaxSize int = 100`

	fmt.Println("=== Test 17: Const Declaration ===")
	ast17 := goparser.Parse(code17)
	fmt.Println(goparser.PrintAST(ast17))

	// Test 18: Grouped const with iota
	code18 := `package main

const (
	Red int = iota
	Green
	Blue
)`

	fmt.Println("=== Test 18: Grouped Const with Iota ===")
	ast18 := goparser.Parse(code18)
	fmt.Println(goparser.PrintAST(ast18))

	// Test 19: Function call (using variables to avoid embedded quotes)
	code19 := `package main

func main() {
	fmt.Println(x, y)
}`

	fmt.Println("=== Test 19: Function Call ===")
	ast19 := goparser.Parse(code19)
	fmt.Println(goparser.PrintAST(ast19))

	// Test 20: Composite literal (struct)
	code20 := `package main

func main() {
	p := Point{X: 1, Y: 2}
}`

	fmt.Println("=== Test 20: Struct Composite Literal ===")
	ast20 := goparser.Parse(code20)
	fmt.Println(goparser.PrintAST(ast20))

	// Test 21: Slice literal
	code21 := `package main

func main() {
	nums := []int{1, 2, 3, 4, 5}
}`

	fmt.Println("=== Test 21: Slice Literal ===")
	ast21 := goparser.Parse(code21)
	fmt.Println(goparser.PrintAST(ast21))

	// Test 22: Map literal (using int keys to avoid embedded quotes)
	code22 := `package main

func main() {
	m := map[int]int{1: 10, 2: 20}
}`

	fmt.Println("=== Test 22: Map Literal ===")
	ast22 := goparser.Parse(code22)
	fmt.Println(goparser.PrintAST(ast22))

	// Test 23: Arithmetic expressions
	code23 := `package main

func main() {
	result := (a + b) * c - d / e
}`

	fmt.Println("=== Test 23: Arithmetic Expressions ===")
	ast23 := goparser.Parse(code23)
	fmt.Println(goparser.PrintAST(ast23))

	// Test 24: Comparison and logical operators
	code24 := `package main

func main() {
	if x > 0 && y < 10 || z == 0 {
		ok = true
	}
}`

	fmt.Println("=== Test 24: Comparison and Logical Ops ===")
	ast24 := goparser.Parse(code24)
	fmt.Println(goparser.PrintAST(ast24))

	// Test 25: Augmented assignment
	code25 := `package main

func main() {
	x += 1
	y -= 2
	z *= 3
}`

	fmt.Println("=== Test 25: Augmented Assignment ===")
	ast25 := goparser.Parse(code25)
	fmt.Println(goparser.PrintAST(ast25))

	// Test 26: Increment and decrement
	code26 := `package main

func main() {
	x++
	y--
}`

	fmt.Println("=== Test 26: Increment/Decrement ===")
	ast26 := goparser.Parse(code26)
	fmt.Println(goparser.PrintAST(ast26))

	// Test 27: Slice expression
	code27 := `package main

func main() {
	a := items[1:3]
	b := items[:5]
	c := items[2:]
}`

	fmt.Println("=== Test 27: Slice Expression ===")
	ast27 := goparser.Parse(code27)
	fmt.Println(goparser.PrintAST(ast27))

	// Test 28: Multiple return values
	code28 := `package main

func divide(a int, b int) (int, error) {
	return a / b, nil
}`

	fmt.Println("=== Test 28: Multiple Return Values ===")
	ast28 := goparser.Parse(code28)
	fmt.Println(goparser.PrintAST(ast28))

	// Test 29: Defer statement
	code29 := `package main

func main() {
	defer cleanup()
}`

	fmt.Println("=== Test 29: Defer Statement ===")
	ast29 := goparser.Parse(code29)
	fmt.Println(goparser.PrintAST(ast29))

	// Test 30: Go statement
	code30 := `package main

func main() {
	go handleRequest(conn)
}`

	fmt.Println("=== Test 30: Go Statement ===")
	ast30 := goparser.Parse(code30)
	fmt.Println(goparser.PrintAST(ast30))

	// Test 31: Generic function
	code31 := `package main

func Max[T Comparable](a T, b T) T {
	return a
}`

	fmt.Println("=== Test 31: Generic Function ===")
	ast31 := goparser.Parse(code31)
	fmt.Println(goparser.PrintAST(ast31))

	// Test 32: Generic type
	code32 := `package main

type Stack[T any] struct {
	items []T
}`

	fmt.Println("=== Test 32: Generic Type ===")
	ast32 := goparser.Parse(code32)
	fmt.Println(goparser.PrintAST(ast32))

	// Test 33: Multi-param generic function
	code33 := `package main

func Map[T any, U any](s []T) []U {
	return nil
}`

	fmt.Println("=== Test 33: Multi-Param Generic Function ===")
	ast33 := goparser.Parse(code33)
	fmt.Println(goparser.PrintAST(ast33))

	// Test 34: Constraint union in interface
	code34 := `package main

type Number interface {
	int | float64
}`

	fmt.Println("=== Test 34: Constraint Union Interface ===")
	ast34 := goparser.Parse(code34)
	fmt.Println(goparser.PrintAST(ast34))

	fmt.Println("=== All Tests Completed ===")
}
