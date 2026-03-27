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

// --- Multi-method interface with void + string return ---

type Logger interface {
	Log(msg string)
	Name() string
}

type ConsoleLogger struct {
	prefix  string
	counter int
}

func (c ConsoleLogger) Log(msg string) {
	// void method — no return
	fmt.Println(c.prefix + ": " + msg)
}

func (c ConsoleLogger) Name() string {
	return c.prefix
}

type SilentLogger struct {
	name string
}

func (s SilentLogger) Log(msg string) {
	// intentionally does nothing
}

func (s SilentLogger) Name() string {
	return s.name
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

// --- Void method test ---

func testVoidMethod() {
	var l Logger = ConsoleLogger{prefix: "TEST", counter: 0}
	l.Log("hello void")
	fmt.Println("PASS: void method")
}

// --- String return test ---

func testStringReturn() {
	var l Logger = ConsoleLogger{prefix: "mylogger", counter: 0}
	n := l.Name()
	if n == "mylogger" {
		fmt.Println("PASS: string return")
	} else {
		panic("FAIL: string return")
	}
}

// --- Multi-method dispatch test ---

func testMultiMethodDispatch() {
	var l Logger = ConsoleLogger{prefix: "MULTI", counter: 0}
	l.Log("step1")
	n := l.Name()
	if n == "MULTI" {
		fmt.Println("PASS: multi-method dispatch")
	} else {
		panic("FAIL: multi-method dispatch")
	}
}

// --- Interface reassignment test ---

func testReassignment() {
	var e Expr = Number{value: 10}
	if e.Eval() != 10 {
		panic("FAIL: reassignment before")
	}
	// Reassign to different concrete type
	var left Expr = Number{value: 3}
	var right Expr = Number{value: 7}
	e = BinOp{op: "+", left: left, right: right}
	if e.Eval() == 10 {
		fmt.Println("PASS: reassignment")
	} else {
		panic("FAIL: reassignment")
	}
}

// --- Logger reassignment to different concrete type ---

func testLoggerReassignment() {
	var l Logger = ConsoleLogger{prefix: "FIRST", counter: 0}
	if l.Name() != "FIRST" {
		panic("FAIL: logger reassignment before")
	}
	l = SilentLogger{name: "SILENT"}
	if l.Name() == "SILENT" {
		fmt.Println("PASS: logger reassignment")
	} else {
		panic("FAIL: logger reassignment")
	}
}

// --- Nil logger test (void method on nil) ---

func testNilLogger() {
	var l Logger
	if l == nil {
		fmt.Println("PASS: nil logger")
	} else {
		panic("FAIL: nil logger")
	}
}

// --- Type switch tests ---

func describeExpr(e Expr) string {
	switch v := e.(type) {
	case Number:
		if v.value >= 0 {
			return "positive-or-zero number"
		}
		return "negative number"
	case BinOp:
		return "binop:" + v.op
	case UnaryOp:
		return "unaryop:" + v.op
	default:
		return "unknown"
	}
}

func testTypeSwitch() {
	var e1 Expr = Number{value: 42}
	if describeExpr(e1) == "positive-or-zero number" {
		fmt.Println("PASS: type switch number")
	} else {
		panic("FAIL: type switch number")
	}

	var left Expr = Number{value: 1}
	var right Expr = Number{value: 2}
	var e2 Expr = BinOp{op: "+", left: left, right: right}
	if describeExpr(e2) == "binop:+" {
		fmt.Println("PASS: type switch binop")
	} else {
		panic("FAIL: type switch binop")
	}

	var operand Expr = Number{value: 5}
	var e3 Expr = UnaryOp{op: "-", operand: operand}
	if describeExpr(e3) == "unaryop:-" {
		fmt.Println("PASS: type switch unaryop")
	} else {
		panic("FAIL: type switch unaryop")
	}
}

func testTypeSwitchNoVar() {
	var e Expr = Number{value: 10}
	result := ""
	switch e.(type) {
	case Number:
		result = "number"
	case BinOp:
		result = "binop"
	default:
		result = "other"
	}
	if result == "number" {
		fmt.Println("PASS: type switch no var")
	} else {
		panic("FAIL: type switch no var")
	}
}

func testTypeSwitchDefault() {
	var left Expr = Number{value: 1}
	var right Expr = Number{value: 2}
	var e Expr = BinOp{op: "+", left: left, right: right}
	result := ""
	switch e.(type) {
	case Number:
		result = "number"
	default:
		result = "not-number"
	}
	if result == "not-number" {
		fmt.Println("PASS: type switch default")
	} else {
		panic("FAIL: type switch default")
	}
}

// --- Multi-value type switch test ---

func classifyExpr(e Expr) string {
	switch e.(type) {
	case Number, UnaryOp:
		return "simple"
	case BinOp:
		return "compound"
	default:
		return "unknown"
	}
}

func testTypeSwitchMultiCase() {
	var e1 Expr = Number{value: 5}
	if classifyExpr(e1) == "simple" {
		fmt.Println("PASS: type switch multi-case number")
	} else {
		panic("FAIL: type switch multi-case number")
	}

	var operand Expr = Number{value: 3}
	var e2 Expr = UnaryOp{op: "-", operand: operand}
	if classifyExpr(e2) == "simple" {
		fmt.Println("PASS: type switch multi-case unaryop")
	} else {
		panic("FAIL: type switch multi-case unaryop")
	}

	var left Expr = Number{value: 1}
	var right Expr = Number{value: 2}
	var e3 Expr = BinOp{op: "+", left: left, right: right}
	if classifyExpr(e3) == "compound" {
		fmt.Println("PASS: type switch multi-case binop")
	} else {
		panic("FAIL: type switch multi-case binop")
	}
}

// --- Interface slice with nil elements test ---

func testInterfaceSliceWithNil() {
	exprs := []Expr{Number{value: 1}, nil, Number{value: 3}}
	if safeEval(exprs[0]) == 1 {
		fmt.Println("PASS: slice nil elem 0")
	} else {
		panic("FAIL: slice nil elem 0")
	}
	if safeEval(exprs[1]) == 0 {
		fmt.Println("PASS: slice nil elem 1")
	} else {
		panic("FAIL: slice nil elem 1")
	}
	if safeEval(exprs[2]) == 3 {
		fmt.Println("PASS: slice nil elem 2")
	} else {
		panic("FAIL: slice nil elem 2")
	}
}

// --- Interface map value test ---

func testInterfaceMap() {
	m := make(map[string]Expr)
	var ea Expr = Number{value: 10}
	var eb Expr = Number{value: 20}
	m["a"] = ea
	m["b"] = eb
	a := m["a"]
	if a.Eval() == 10 {
		fmt.Println("PASS: map value a")
	} else {
		panic("FAIL: map value a")
	}
	b := m["b"]
	if b.Eval() == 20 {
		fmt.Println("PASS: map value b")
	} else {
		panic("FAIL: map value b")
	}
}

// --- Methods on non-struct types ---

type ConstExpr int

func (c ConstExpr) Eval() int {
	return int(c)
}

func testNonStructMethod() {
	var e Expr = ConstExpr(42)
	if e.Eval() == 42 {
		fmt.Println("PASS: non-struct method")
	} else {
		panic("FAIL: non-struct method")
	}
}

// --- Interface variadic params test ---

func sumExprs(exprs ...Expr) int {
	total := 0
	for i := 0; i < len(exprs); i++ {
		total = total + exprs[i].Eval()
	}
	return total
}

func testVariadicInterface() {
	var a Expr = Number{value: 10}
	var b Expr = Number{value: 20}
	var c Expr = Number{value: 30}
	result := sumExprs(a, b, c)
	if result == 60 {
		fmt.Println("PASS: variadic interface")
	} else {
		panic("FAIL: variadic interface")
	}
}

// --- Interface composition (embedded interfaces) ---

type Evaler interface {
	Eval() int
}

type Stringer interface {
	String() string
}

type ExprFull interface {
	Evaler
	Stringer
}

type RichNumber struct {
	value int
	label string
}

func (r RichNumber) Eval() int {
	return r.value
}

func (r RichNumber) String() string {
	return r.label
}

func testInterfaceComposition() {
	var ef ExprFull = RichNumber{value: 100, label: "hundred"}
	if ef.Eval() == 100 {
		fmt.Println("PASS: composition eval")
	} else {
		panic("FAIL: composition eval")
	}
	if ef.String() == "hundred" {
		fmt.Println("PASS: composition string")
	} else {
		panic("FAIL: composition string")
	}
}

func testComposedAsParam(ef ExprFull) int {
	return ef.Eval()
}

func testComposedParam() {
	var ef ExprFull = RichNumber{value: 55, label: "fifty-five"}
	result := testComposedAsParam(ef)
	if result == 55 {
		fmt.Println("PASS: composed as param")
	} else {
		panic("FAIL: composed as param")
	}
}

func makeExprFull(val int, lbl string) ExprFull {
	var ef ExprFull = RichNumber{value: val, label: lbl}
	return ef
}

func testComposedReturn() {
	ef := makeExprFull(77, "seventy-seven")
	if ef.Eval() == 77 {
		fmt.Println("PASS: composed return eval")
	} else {
		panic("FAIL: composed return eval")
	}
	if ef.String() == "seventy-seven" {
		fmt.Println("PASS: composed return string")
	} else {
		panic("FAIL: composed return string")
	}
}

func evalAny(e Evaler) int {
	return e.Eval()
}

func testComposedToEmbedded() {
	var ef ExprFull = RichNumber{value: 33, label: "thirty-three"}
	result := evalAny(ef)
	if result == 33 {
		fmt.Println("PASS: composed to embedded")
	} else {
		panic("FAIL: composed to embedded")
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
	testVoidMethod()
	testStringReturn()
	testMultiMethodDispatch()
	testReassignment()
	testLoggerReassignment()
	testNilLogger()
	testTypeSwitch()
	testTypeSwitchNoVar()
	testTypeSwitchDefault()
	testTypeSwitchMultiCase()
	testInterfaceSliceWithNil()
	testInterfaceMap()
	testVariadicInterface()
	testNonStructMethod()
	testInterfaceComposition()
	testComposedParam()
	testComposedReturn()
	testComposedToEmbedded()
}
