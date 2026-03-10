package rustparser

// Node types
const NodeModule int = 0
const NodeFnDef int = 1
const NodeParam int = 2
const NodeBlock int = 3
const NodeLetBinding int = 4
const NodeIf int = 5
const NodeWhile int = 6
const NodeReturn int = 7
const NodeBinOp int = 8
const NodeCall int = 9
const NodeName int = 10
const NodeNum int = 11
const NodeStr int = 12
const NodeBool int = 13
const NodeAssign int = 14
const NodeStructDef int = 15
const NodeField int = 16
const NodeExprStmt int = 17
const NodeUnaryOp int = 18
const NodeFieldAccess int = 19
const NodeImplBlock int = 20
const NodeTypeRef int = 21
const NodeForLoop int = 22
const NodeBreak int = 23
const NodeContinue int = 24
const NodeLoop int = 25

// Node represents an AST node using slice-based children
type Node struct {
	Type     int
	Name     string
	Value    string
	Op       string
	Children []Node
	Line     int
}

// NewNode creates a new node with the given type
func NewNode(nodeType int) Node {
	return Node{Type: nodeType}
}

// NewNodeWithName creates a new node with the given type and name
func NewNodeWithName(nodeType int, name string) Node {
	return Node{Type: nodeType, Name: name}
}

// NewNodeWithValue creates a new node with the given type and value
func NewNodeWithValue(nodeType int, value string) Node {
	return Node{Type: nodeType, Value: value}
}

// NewNodeWithOp creates a new node with the given type and operator
func NewNodeWithOp(nodeType int, op string) Node {
	return Node{Type: nodeType, Op: op}
}

// AddChild adds a child node and returns the modified parent
func AddChild(parent Node, child Node) Node {
	parent.Children = append(parent.Children, child)
	return parent
}

// SetLine sets the line number and returns the modified node
func SetLine(node Node, line int) Node {
	node.Line = line
	return node
}

// NodeTypeName returns a human-readable name for node type
func NodeTypeName(nodeType int) string {
	if nodeType == NodeModule {
		return "Module"
	}
	if nodeType == NodeFnDef {
		return "FnDef"
	}
	if nodeType == NodeParam {
		return "Param"
	}
	if nodeType == NodeBlock {
		return "Block"
	}
	if nodeType == NodeLetBinding {
		return "LetBinding"
	}
	if nodeType == NodeIf {
		return "If"
	}
	if nodeType == NodeWhile {
		return "While"
	}
	if nodeType == NodeReturn {
		return "Return"
	}
	if nodeType == NodeBinOp {
		return "BinOp"
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
	if nodeType == NodeBool {
		return "Bool"
	}
	if nodeType == NodeAssign {
		return "Assign"
	}
	if nodeType == NodeStructDef {
		return "StructDef"
	}
	if nodeType == NodeField {
		return "Field"
	}
	if nodeType == NodeExprStmt {
		return "ExprStmt"
	}
	if nodeType == NodeUnaryOp {
		return "UnaryOp"
	}
	if nodeType == NodeFieldAccess {
		return "FieldAccess"
	}
	if nodeType == NodeImplBlock {
		return "ImplBlock"
	}
	if nodeType == NodeTypeRef {
		return "TypeRef"
	}
	if nodeType == NodeForLoop {
		return "ForLoop"
	}
	if nodeType == NodeBreak {
		return "Break"
	}
	if nodeType == NodeContinue {
		return "Continue"
	}
	if nodeType == NodeLoop {
		return "Loop"
	}
	return "Unknown"
}
