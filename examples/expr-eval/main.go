package main

import "fmt"

// --- Interface ---

type Expr interface {
	Eval() int
}

// --- Concrete types ---

type Number struct {
	value int
}

func (n Number) Eval() int {
	return n.value
}

type BinOp struct {
	op    string
	left  Expr
	right Expr
}

func (b BinOp) Eval() int {
	l := b.left.Eval()
	r := b.right.Eval()
	if b.op == "+" {
		return l + r
	}
	if b.op == "-" {
		return l - r
	}
	if b.op == "*" {
		return l * r
	}
	return 0
}

type UnaryOp struct {
	op      string
	operand Expr
}

func (u UnaryOp) Eval() int {
	v := u.operand.Eval()
	if u.op == "-" {
		return -v
	}
	return v
}

// --- Helper functions ---

func evalExpr(e Expr) int {
	return e.Eval()
}

func makeNumber(val int) Expr {
	var result Expr = Number{value: val}
	return result
}

func safeEval(e Expr) int {
	if e == nil {
		return 0
	}
	return e.Eval()
}

// --- Test functions ---

func testBasicEval() {
	var e Expr = Number{value: 42}
	if e.Eval() == 42 {
		fmt.Println("PASS: basic eval")
	} else {
		panic("FAIL: basic eval")
	}
}

func testBinOpAdd() {
	var left Expr = Number{value: 3}
	var right Expr = Number{value: 4}
	var e Expr = BinOp{op: "+", left: left, right: right}
	if e.Eval() == 7 {
		fmt.Println("PASS: binop add")
	} else {
		panic("FAIL: binop add")
	}
}

func testBinOpMul() {
	var left Expr = Number{value: 5}
	var right Expr = Number{value: 6}
	var e Expr = BinOp{op: "*", left: left, right: right}
	if e.Eval() == 30 {
		fmt.Println("PASS: binop mul")
	} else {
		panic("FAIL: binop mul")
	}
}

func testNestedExpr() {
	// (3 + 4) * 2 = 14
	var three Expr = Number{value: 3}
	var four Expr = Number{value: 4}
	var sum Expr = BinOp{op: "+", left: three, right: four}
	var two Expr = Number{value: 2}
	var product Expr = BinOp{op: "*", left: sum, right: two}
	if product.Eval() == 14 {
		fmt.Println("PASS: nested expr")
	} else {
		panic("FAIL: nested expr")
	}
}

func testUnaryNeg() {
	var operand Expr = Number{value: 10}
	var e Expr = UnaryOp{op: "-", operand: operand}
	if e.Eval() == -10 {
		fmt.Println("PASS: unary neg")
	} else {
		panic("FAIL: unary neg")
	}
}

func testComplexExpr() {
	// -(3 + 4) * (10 - 2) = -7 * 8 = -56
	var three Expr = Number{value: 3}
	var four Expr = Number{value: 4}
	var sum Expr = BinOp{op: "+", left: three, right: four}
	var negSum Expr = UnaryOp{op: "-", operand: sum}
	var ten Expr = Number{value: 10}
	var two Expr = Number{value: 2}
	var diff Expr = BinOp{op: "-", left: ten, right: two}
	var result Expr = BinOp{op: "*", left: negSum, right: diff}
	if result.Eval() == -56 {
		fmt.Println("PASS: complex expr")
	} else {
		panic("FAIL: complex expr")
	}
}

func testTypeAssert() {
	var e Expr = Number{value: 99}
	n := e.(Number)
	if n.value == 99 {
		fmt.Println("PASS: type assert")
	} else {
		panic("FAIL: type assert")
	}
}

func testCommaOk() {
	var e Expr = Number{value: 77}
	n, ok := e.(Number)
	if ok {
		if n.value == 77 {
			fmt.Println("PASS: comma-ok success")
		} else {
			panic("FAIL: comma-ok success value")
		}
	} else {
		panic("FAIL: comma-ok success")
	}

	// Failed assertion: BinOp is not Number
	var left Expr = Number{value: 1}
	var right Expr = Number{value: 2}
	var e2 Expr = BinOp{op: "+", left: left, right: right}
	_, ok2 := e2.(Number)
	if ok2 == false {
		fmt.Println("PASS: comma-ok failure")
	} else {
		panic("FAIL: comma-ok failure")
	}
}

func testExprAsParam() {
	var e Expr = Number{value: 55}
	if evalExpr(e) == 55 {
		fmt.Println("PASS: expr as param")
	} else {
		panic("FAIL: expr as param")
	}
}

func testExprAsReturn() {
	e := makeNumber(33)
	if e.Eval() == 33 {
		fmt.Println("PASS: expr as return")
	} else {
		panic("FAIL: expr as return")
	}
}

func testNilExpr() {
	var e Expr = nil
	if e == nil {
		fmt.Println("PASS: nil check")
	} else {
		panic("FAIL: nil check")
	}

	result := safeEval(e)
	if result == 0 {
		fmt.Println("PASS: safe eval nil")
	} else {
		panic("FAIL: safe eval nil")
	}
}

func testExprSlice() {
	var e1 Expr = Number{value: 1}
	var e2 Expr = Number{value: 2}
	var e3 Expr = Number{value: 3}
	var sum Expr = BinOp{op: "+", left: e1, right: e2}
	exprs := []Expr{e1, e2, e3, sum}
	total := 0
	for i := 0; i < len(exprs); i++ {
		total = total + exprs[i].Eval()
	}
	// 1 + 2 + 3 + (1+2) = 9
	if total == 9 {
		fmt.Println("PASS: expr slice")
	} else {
		panic("FAIL: expr slice")
	}
}

func main() {
	testBasicEval()
	testBinOpAdd()
	testBinOpMul()
	testNestedExpr()
	testUnaryNeg()
	testComplexExpr()
	testTypeAssert()
	testCommaOk()
	testExprAsParam()
	testExprAsReturn()
	testNilExpr()
	testExprSlice()
}
