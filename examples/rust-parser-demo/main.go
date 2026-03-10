package main

import (
	"fmt"
	"libs/rustparser"
)

// PtrNode represents an AST node using pointer-based tree structure.
// Uses first-child/next-sibling representation for N-ary tree.
type PtrNode struct {
	Type       int
	Name       string
	Value      string
	Op         string
	FirstChild *PtrNode
	NextSib    *PtrNode
	Line       int
}

// convertNode converts a slice-based rustparser.Node to a pointer-based PtrNode tree.
func convertNode(n rustparser.Node) *PtrNode {
	pn := &PtrNode{
		Type:       n.Type,
		Name:       n.Name,
		Value:      n.Value,
		Op:         n.Op,
		FirstChild: nil,
		NextSib:    nil,
		Line:       n.Line,
	}
	if len(n.Children) == 0 {
		return pn
	}
	// Set first child
	pn.FirstChild = convertNode(n.Children[0])
	// Link remaining children as siblings
	curr := pn.FirstChild
	i := 1
	for i < len(n.Children) {
		curr.NextSib = convertNode(n.Children[i])
		curr = curr.NextSib
		i = i + 1
	}
	return pn
}

// countNodes counts total nodes in the pointer tree.
func countNodes(n *PtrNode) int {
	if n == nil {
		return 0
	}
	count := 1
	child := n.FirstChild
	for child != nil {
		count = count + countNodes(child)
		child = child.NextSib
	}
	return count
}

// countSliceNodes counts total nodes in the original slice-based tree.
func countSliceNodes(n rustparser.Node) int {
	count := 1
	i := 0
	for i < len(n.Children) {
		count = count + countSliceNodes(n.Children[i])
		i = i + 1
	}
	return count
}

// nodeDepth computes the maximum depth of the pointer tree.
func nodeDepth(n *PtrNode) int {
	if n == nil {
		return 0
	}
	maxChildD := 0
	child := n.FirstChild
	for child != nil {
		d := nodeDepth(child)
		if d > maxChildD {
			maxChildD = d
		}
		child = child.NextSib
	}
	return 1 + maxChildD
}

// findByType finds the first node with the given type using DFS.
func findByType(n *PtrNode, nodeType int) *PtrNode {
	if n == nil {
		return nil
	}
	if n.Type == nodeType {
		return n
	}
	child := n.FirstChild
	for child != nil {
		found := findByType(child, nodeType)
		if found != nil {
			return found
		}
		child = child.NextSib
	}
	return nil
}

// countByType counts how many nodes of a given type exist.
func countByType(n *PtrNode, nodeType int) int {
	if n == nil {
		return 0
	}
	count := 0
	if n.Type == nodeType {
		count = 1
	}
	child := n.FirstChild
	for child != nil {
		count = count + countByType(child, nodeType)
		child = child.NextSib
	}
	return count
}

// collectNames collects all Name node values into a slice.
func collectNames(n *PtrNode, names []string) []string {
	if n == nil {
		return names
	}
	if n.Type == rustparser.NodeName {
		names = append(names, n.Name)
	}
	child := n.FirstChild
	for child != nil {
		names = collectNames(child, names)
		child = child.NextSib
	}
	return names
}

// childCount counts direct children of a pointer node.
func childCount(n *PtrNode) int {
	if n == nil {
		return 0
	}
	count := 0
	child := n.FirstChild
	for child != nil {
		count = count + 1
		child = child.NextSib
	}
	return count
}

// nthChild returns the nth direct child (0-indexed).
func nthChild(n *PtrNode, idx int) *PtrNode {
	if n == nil {
		return nil
	}
	child := n.FirstChild
	i := 0
	for child != nil {
		if i == idx {
			return child
		}
		child = child.NextSib
		i = i + 1
	}
	return nil
}

// findByName finds the first node with a given name using DFS.
func findByName(n *PtrNode, name string) *PtrNode {
	if n == nil {
		return nil
	}
	if n.Name == name {
		return n
	}
	child := n.FirstChild
	for child != nil {
		found := findByName(child, name)
		if found != nil {
			return found
		}
		child = child.NextSib
	}
	return nil
}

// printIndent prints indentation spaces.
func printIndent(indent int) {
	i := 0
	for i < indent {
		fmt.Print("  ")
		i = i + 1
	}
}

// printNode prints a pointer-based tree with indentation.
func printNode(n *PtrNode, indent int) {
	if n == nil {
		return
	}
	printIndent(indent)
	fmt.Print(rustparser.NodeTypeName(n.Type))
	if n.Name != "" {
		fmt.Print(" name=")
		fmt.Print(n.Name)
	}
	if n.Value != "" {
		fmt.Print(" value=")
		fmt.Print(n.Value)
	}
	if n.Op != "" {
		fmt.Print(" op=")
		fmt.Print(n.Op)
	}
	fmt.Println("")
	child := n.FirstChild
	for child != nil {
		printNode(child, indent+1)
		child = child.NextSib
	}
}

func main() {
	// Test 1: Parse function definition, convert to pointer tree
	code1 := `fn add(a: i32, b: i32) -> i32 {
    return a + b;
}`
	fmt.Println("=== Test 1: Function Definition ===")
	ast1 := rustparser.Parse(code1)
	tree1 := convertNode(ast1)
	printNode(tree1, 0)
	sliceCount1 := countSliceNodes(ast1)
	ptrCount1 := countNodes(tree1)
	if sliceCount1 == ptrCount1 {
		fmt.Println("PASS: node count matches")
	} else {
		panic("FAIL: node count mismatch")
	}

	// Test 2: Parse struct definition
	code2 := `struct Point {
    x: i32,
    y: i32,
}`
	fmt.Println("=== Test 2: Struct Definition ===")
	ast2 := rustparser.Parse(code2)
	tree2 := convertNode(ast2)
	printNode(tree2, 0)
	structNode := findByType(tree2, rustparser.NodeStructDef)
	if structNode != nil && structNode.Name == "Point" {
		fmt.Println("PASS: found struct Point")
	} else {
		panic("FAIL: struct not found")
	}
	fieldCount := countByType(tree2, rustparser.NodeField)
	if fieldCount == 2 {
		fmt.Println("PASS: field count")
	} else {
		panic("FAIL: field count")
	}

	// Test 3: Parse let bindings, test sibling chain
	code3 := `let x = 5;
let mut y = 10;
let z = x + y;`
	fmt.Println("=== Test 3: Let Bindings (Sibling Chain) ===")
	ast3 := rustparser.Parse(code3)
	tree3 := convertNode(ast3)
	printNode(tree3, 0)
	// Count direct children of module (3 let bindings)
	sibCount := childCount(tree3)
	if sibCount == 3 {
		fmt.Println("PASS: sibling count")
	} else {
		panic("FAIL: sibling count")
	}
	letCount := countByType(tree3, rustparser.NodeLetBinding)
	if letCount == 3 {
		fmt.Println("PASS: let binding count")
	} else {
		panic("FAIL: let binding count")
	}

	// Test 4: If/else, test tree depth and structure
	code4 := `if x > 0 {
    y = x;
} else {
    y = 0;
}`
	fmt.Println("=== Test 4: If/Else ===")
	ast4 := rustparser.Parse(code4)
	tree4 := convertNode(ast4)
	printNode(tree4, 0)
	sliceCount4 := countSliceNodes(ast4)
	ptrCount4 := countNodes(tree4)
	if sliceCount4 == ptrCount4 {
		fmt.Println("PASS: if/else node count")
	} else {
		panic("FAIL: if/else node count")
	}
	ifNode := findByType(tree4, rustparser.NodeIf)
	if ifNode != nil {
		fmt.Println("PASS: found If node")
	} else {
		panic("FAIL: If not found")
	}

	// Test 5: While loop
	code5 := `while n > 0 {
    n = n - 1;
}`
	fmt.Println("=== Test 5: While Loop ===")
	ast5 := rustparser.Parse(code5)
	tree5 := convertNode(ast5)
	printNode(tree5, 0)
	sliceCount5 := countSliceNodes(ast5)
	ptrCount5 := countNodes(tree5)
	if sliceCount5 == ptrCount5 {
		fmt.Println("PASS: while node count")
	} else {
		panic("FAIL: while node count")
	}
	whileNode := findByType(tree5, rustparser.NodeWhile)
	if whileNode != nil {
		fmt.Println("PASS: found While node")
	} else {
		panic("FAIL: While not found")
	}

	// Test 6: Find node by type, returns pointer from deep in tree
	fmt.Println("=== Test 6: Find Nodes By Type ===")
	fnNode := findByType(tree1, rustparser.NodeFnDef)
	if fnNode != nil && fnNode.Name == "add" {
		fmt.Println("PASS: find FnDef add")
	} else {
		panic("FAIL: find FnDef")
	}
	retNode := findByType(tree1, rustparser.NodeReturn)
	if retNode != nil {
		fmt.Println("PASS: find Return")
	} else {
		panic("FAIL: find Return")
	}
	binOpNode := findByType(tree1, rustparser.NodeBinOp)
	if binOpNode != nil && binOpNode.Op == "+" {
		fmt.Println("PASS: find BinOp +")
	} else {
		panic("FAIL: find BinOp")
	}

	// Test 7: Modify node through pointer - test mutation persistence
	code7 := `let val = 42;`
	fmt.Println("=== Test 7: Modify Through Pointer ===")
	ast7 := rustparser.Parse(code7)
	tree7 := convertNode(ast7)
	numNode := findByType(tree7, rustparser.NodeNum)
	if numNode != nil {
		fmt.Print("Before: ")
		fmt.Println(numNode.Value)
		numNode.Value = "99"
		fmt.Print("After: ")
		fmt.Println(numNode.Value)
		numNode2 := findByType(tree7, rustparser.NodeNum)
		if numNode2 != nil && numNode2.Value == "99" {
			fmt.Println("PASS: modify through pointer persists")
		} else {
			panic("FAIL: modify through pointer")
		}
	} else {
		panic("FAIL: could not find Num node")
	}

	// Test 8: Nested expression tree depth
	code8 := `let result = (a + b) * c;`
	fmt.Println("=== Test 8: Nested Expression Depth ===")
	ast8 := rustparser.Parse(code8)
	tree8 := convertNode(ast8)
	printNode(tree8, 0)
	depth8 := nodeDepth(tree8)
	fmt.Print("Depth: ")
	fmt.Println(depth8)
	if depth8 >= 4 {
		fmt.Println("PASS: nested expression depth")
	} else {
		panic("FAIL: nested expression depth")
	}

	// Test 9: Leaf node has nil child pointers
	code9 := `let leaf = 1;`
	fmt.Println("=== Test 9: Leaf Node (Nil Children) ===")
	ast9 := rustparser.Parse(code9)
	tree9 := convertNode(ast9)
	leafNum := findByType(tree9, rustparser.NodeNum)
	if leafNum != nil && leafNum.FirstChild == nil {
		fmt.Println("PASS: leaf node has nil children")
	} else {
		panic("FAIL: leaf node children")
	}

	// Test 10: Collect names from tree
	code10 := `let z = a + b + c;`
	fmt.Println("=== Test 10: Collect Names ===")
	ast10 := rustparser.Parse(code10)
	tree10 := convertNode(ast10)
	names := []string{}
	names = collectNames(tree10, names)
	fmt.Print("Name count: ")
	fmt.Println(len(names))
	if len(names) > 0 {
		fmt.Println("PASS: collect names")
	} else {
		panic("FAIL: collect names")
	}

	// Test 11: Count BinOp nodes across multiple expressions
	code11 := `let a = x + y;
let b = p - q;
let c = m * n;`
	fmt.Println("=== Test 11: Count BinOps ===")
	ast11 := rustparser.Parse(code11)
	tree11 := convertNode(ast11)
	binOpCount := countByType(tree11, rustparser.NodeBinOp)
	if binOpCount == 3 {
		fmt.Println("PASS: binop count")
	} else {
		panic("FAIL: binop count")
	}

	// Test 12: Impl block with methods
	code12 := `impl Point {
    fn new(x: i32, y: i32) -> Point {
        return x;
    }
    fn sum(self) -> i32 {
        return 0;
    }
}`
	fmt.Println("=== Test 12: Impl Block ===")
	ast12 := rustparser.Parse(code12)
	tree12 := convertNode(ast12)
	printNode(tree12, 0)
	implNode := findByType(tree12, rustparser.NodeImplBlock)
	if implNode != nil && implNode.Name == "Point" {
		fmt.Println("PASS: found impl Point")
	} else {
		panic("FAIL: impl not found")
	}
	fnCount := countByType(tree12, rustparser.NodeFnDef)
	if fnCount == 2 {
		fmt.Println("PASS: method count in impl")
	} else {
		panic("FAIL: method count")
	}

	// Test 13: nthChild traversal
	fmt.Println("=== Test 13: nthChild Traversal ===")
	firstLet := nthChild(tree3, 0)
	if firstLet != nil && firstLet.Name == "x" {
		fmt.Println("PASS: first let is x")
	} else {
		panic("FAIL: nthChild 0")
	}
	secondLet := nthChild(tree3, 1)
	if secondLet != nil && secondLet.Name == "y" {
		fmt.Println("PASS: second let is y")
	} else {
		panic("FAIL: nthChild 1")
	}
	thirdLet := nthChild(tree3, 2)
	if thirdLet != nil && thirdLet.Name == "z" {
		fmt.Println("PASS: third let is z")
	} else {
		panic("FAIL: nthChild 2")
	}
	outOfBounds := nthChild(tree3, 99)
	if outOfBounds == nil {
		fmt.Println("PASS: nthChild out of bounds returns nil")
	} else {
		panic("FAIL: nthChild out of bounds")
	}

	// Test 14: findByName traversal
	fmt.Println("=== Test 14: Find By Name ===")
	addFn := findByName(tree1, "add")
	if addFn != nil {
		fmt.Println("PASS: found node named add")
	} else {
		panic("FAIL: findByName add")
	}
	paramA := findByName(tree1, "a")
	if paramA != nil {
		fmt.Println("PASS: found node named a")
	} else {
		panic("FAIL: findByName a")
	}
	noSuch := findByName(tree1, "nonexistent")
	if noSuch == nil {
		fmt.Println("PASS: findByName returns nil for missing")
	} else {
		panic("FAIL: findByName nonexistent")
	}

	// Test 15: Complex program with multiple constructs
	code15 := `struct Rect {
    w: i32,
    h: i32,
}
fn area(r: Rect) -> i32 {
    return r.w * r.h;
}
fn main() {
    let mut total = 0;
    let n = 5;
    while n > 0 {
        total = total + n;
    }
    if total > 10 {
        total = 10;
    } else {
        total = 0;
    }
}`
	fmt.Println("=== Test 15: Complex Program ===")
	ast15 := rustparser.Parse(code15)
	tree15 := convertNode(ast15)
	sliceCount15 := countSliceNodes(ast15)
	ptrCount15 := countNodes(tree15)
	if sliceCount15 == ptrCount15 {
		fmt.Println("PASS: complex program node count")
	} else {
		panic("FAIL: complex program node count")
	}
	depth15 := nodeDepth(tree15)
	if depth15 >= 5 {
		fmt.Println("PASS: complex program depth")
	} else {
		panic("FAIL: complex program depth")
	}
	structCount := countByType(tree15, rustparser.NodeStructDef)
	fnDefCount := countByType(tree15, rustparser.NodeFnDef)
	if structCount == 1 && fnDefCount == 2 {
		fmt.Println("PASS: complex program item counts")
	} else {
		panic("FAIL: complex program item counts")
	}

	// Test 16: Pointer field mutation propagation through tree
	fmt.Println("=== Test 16: Deep Mutation Propagation ===")
	code16 := `fn foo(x: i32) -> i32 {
    return x + 1;
}`
	ast16 := rustparser.Parse(code16)
	tree16 := convertNode(ast16)
	// Find the return node and modify its child
	ret16 := findByType(tree16, rustparser.NodeReturn)
	if ret16 != nil && ret16.FirstChild != nil {
		ret16.FirstChild.Op = "modified"
		// Re-find and verify
		ret16b := findByType(tree16, rustparser.NodeReturn)
		if ret16b != nil && ret16b.FirstChild != nil && ret16b.FirstChild.Op == "modified" {
			fmt.Println("PASS: deep mutation propagation")
		} else {
			panic("FAIL: deep mutation not visible")
		}
	} else {
		panic("FAIL: return node structure")
	}

	// Test 17: Multiple pointer aliases to same tree node
	fmt.Println("=== Test 17: Multiple Pointer Aliases ===")
	code17 := `let v = 100;`
	ast17 := rustparser.Parse(code17)
	tree17 := convertNode(ast17)
	alias1 := findByType(tree17, rustparser.NodeNum)
	alias2 := findByType(tree17, rustparser.NodeNum)
	if alias1 != nil && alias2 != nil {
		alias1.Value = "changed"
		if alias2.Value == "changed" {
			fmt.Println("PASS: pointer aliases share state")
		} else {
			panic("FAIL: aliases not shared")
		}
	} else {
		panic("FAIL: aliases nil")
	}

	// Test 18: Tree with for loop
	code18 := `for i in items {
    total = total + i;
}`
	fmt.Println("=== Test 18: For Loop ===")
	ast18 := rustparser.Parse(code18)
	tree18 := convertNode(ast18)
	printNode(tree18, 0)
	sliceCount18 := countSliceNodes(ast18)
	ptrCount18 := countNodes(tree18)
	if sliceCount18 == ptrCount18 {
		fmt.Println("PASS: for loop node count")
	} else {
		panic("FAIL: for loop node count")
	}
	forNode := findByType(tree18, rustparser.NodeForLoop)
	if forNode != nil && forNode.Name == "i" {
		fmt.Println("PASS: for loop iterator name")
	} else {
		panic("FAIL: for loop structure")
	}

	fmt.Println("=== All rust parser pointer tests passed ===")
}
