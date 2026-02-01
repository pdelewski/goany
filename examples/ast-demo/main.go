package main

import "fmt"

// --- Node type tags ---

const NodeNum int = 1
const NodeAdd int = 2
const NodeSub int = 3
const NodeMul int = 4

// Node represents an expression tree node (tagged union style)
type Node struct {
	Tag   int
	Value interface{} // NodeNum: holds the int value
	Left  int         // child index (-1 = none)
	Right int         // child index (-1 = none)
}

// --- AST builder ---

func addNum(nodes []Node, val int) ([]Node, int) {
	idx := len(nodes)
	var n Node
	n.Tag = NodeNum
	n.Value = val
	n.Left = -1
	n.Right = -1
	nodes = append(nodes, n)
	return nodes, idx
}

func addBinOp(nodes []Node, tag int, left int, right int) ([]Node, int) {
	idx := len(nodes)
	var n Node
	n.Tag = tag
	n.Value = 0
	n.Left = left
	n.Right = right
	nodes = append(nodes, n)
	return nodes, idx
}

// --- Evaluator ---

func evalNode(nodes []Node, idx int) int {
	tag := nodes[idx].Tag
	if tag == NodeNum {
		return nodes[idx].Value.(int)
	}
	leftVal := evalNode(nodes, nodes[idx].Left)
	rightVal := evalNode(nodes, nodes[idx].Right)
	if tag == NodeAdd {
		return leftVal + rightVal
	}
	if tag == NodeSub {
		return leftVal - rightVal
	}
	if tag == NodeMul {
		return leftVal * rightVal
	}
	return 0
}

// --- Printer ---

func tagName(tag int) string {
	if tag == NodeAdd {
		return "+"
	}
	if tag == NodeSub {
		return "-"
	}
	if tag == NodeMul {
		return "*"
	}
	return "?"
}

func printExpr(nodes []Node, idx int) string {
	tag := nodes[idx].Tag
	if tag == NodeNum {
		return intToStr(nodes[idx].Value.(int))
	}
	left := printExpr(nodes, nodes[idx].Left)
	right := printExpr(nodes, nodes[idx].Right)
	return "(" + left + " " + tagName(tag) + " " + right + ")"
}

// --- Helpers ---

func intToStr(n int) string {
	digits := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	result := ""
	for n > 0 {
		d := n % 10
		result = digits[d] + result
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// --- Main ---

func main() {
	var nodes []Node

	// Build AST for: (3 + 4) * (10 - 2)
	var i3 int
	var i4 int
	var i10 int
	var i2 int
	var iAdd int
	var iSub int
	var iMul int

	nodes, i3 = addNum(nodes, 3)
	nodes, i4 = addNum(nodes, 4)
	nodes, i10 = addNum(nodes, 10)
	nodes, i2 = addNum(nodes, 2)
	nodes, iAdd = addBinOp(nodes, NodeAdd, i3, i4)
	nodes, iSub = addBinOp(nodes, NodeSub, i10, i2)
	nodes, iMul = addBinOp(nodes, NodeMul, iAdd, iSub)

	fmt.Println(printExpr(nodes, iMul))
	fmt.Println(evalNode(nodes, iMul))

	// Build AST for: (5 * 3) + 1
	var i5 int
	var i1 int
	var iMul2 int
	var iAdd2 int

	nodes, i5 = addNum(nodes, 5)
	nodes, i1 = addNum(nodes, 1)
	nodes, iMul2 = addBinOp(nodes, NodeMul, i5, i3)
	nodes, iAdd2 = addBinOp(nodes, NodeAdd, iMul2, i1)

	fmt.Println(printExpr(nodes, iAdd2))
	fmt.Println(evalNode(nodes, iAdd2))
}
