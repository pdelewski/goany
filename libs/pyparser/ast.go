package pyparser

// Node types
const NodeModule int = 1
const NodeFunctionDef int = 2
const NodeReturn int = 3
const NodeIf int = 4
const NodeFor int = 5
const NodeWhile int = 6
const NodeAssign int = 7
const NodeExpr int = 8
const NodeBinOp int = 9
const NodeUnaryOp int = 10
const NodeCall int = 11
const NodeName int = 12
const NodeNum int = 13
const NodeStr int = 14
const NodeList int = 15
const NodeSubscript int = 16
const NodePass int = 17
const NodeBreak int = 18
const NodeContinue int = 19
const NodeBool int = 20
const NodeNone int = 21
const NodeCompare int = 22
const NodeElif int = 23
const NodeElse int = 24

// Node represents an AST node
type Node struct {
	Type     int
	Name     string // For identifiers, function names
	Value    string // For literals
	Op       string // For operators
	Children []Node // Child nodes
	Line     int
}

// NewNode creates a new AST node
func NewNode(nodeType int) Node {
	return Node{
		Type:     nodeType,
		Name:     "",
		Value:    "",
		Op:       "",
		Children: []Node{},
		Line:     0,
	}
}

// NewNodeWithName creates a node with a name
func NewNodeWithName(nodeType int, name string) Node {
	node := NewNode(nodeType)
	node.Name = name
	return node
}

// NewNodeWithValue creates a node with a value
func NewNodeWithValue(nodeType int, value string) Node {
	node := NewNode(nodeType)
	node.Value = value
	return node
}

// NewNodeWithOp creates a node with an operator
func NewNodeWithOp(nodeType int, op string) Node {
	node := NewNode(nodeType)
	node.Op = op
	return node
}

// AddChild adds a child node
func AddChild(parent Node, child Node) Node {
	parent.Children = append(parent.Children, child)
	return parent
}

// SetLine sets the line number
func SetLine(node Node, line int) Node {
	node.Line = line
	return node
}

// NodeTypeName returns a human-readable name for node type
func NodeTypeName(nodeType int) string {
	if nodeType == NodeModule {
		return "Module"
	}
	if nodeType == NodeFunctionDef {
		return "FunctionDef"
	}
	if nodeType == NodeReturn {
		return "Return"
	}
	if nodeType == NodeIf {
		return "If"
	}
	if nodeType == NodeFor {
		return "For"
	}
	if nodeType == NodeWhile {
		return "While"
	}
	if nodeType == NodeAssign {
		return "Assign"
	}
	if nodeType == NodeExpr {
		return "Expr"
	}
	if nodeType == NodeBinOp {
		return "BinOp"
	}
	if nodeType == NodeUnaryOp {
		return "UnaryOp"
	}
	if nodeType == NodeCall {
		return "Call"
	}
	if nodeType == NodeName {
		return "Name"
	}
	if nodeType == NodeNum {
		return "Num"
	}
	if nodeType == NodeStr {
		return "Str"
	}
	if nodeType == NodeList {
		return "List"
	}
	if nodeType == NodeSubscript {
		return "Subscript"
	}
	if nodeType == NodePass {
		return "Pass"
	}
	if nodeType == NodeBreak {
		return "Break"
	}
	if nodeType == NodeContinue {
		return "Continue"
	}
	if nodeType == NodeBool {
		return "Bool"
	}
	if nodeType == NodeNone {
		return "None"
	}
	if nodeType == NodeCompare {
		return "Compare"
	}
	if nodeType == NodeElif {
		return "Elif"
	}
	if nodeType == NodeElse {
		return "Else"
	}
	return "Unknown"
}
