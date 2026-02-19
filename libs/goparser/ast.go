package goparser

// Node types
const NodeFile int = 1
const NodePackage int = 2
const NodeImport int = 3
const NodeImportSpec int = 4
const NodeFuncDecl int = 5
const NodeMethodDecl int = 6
const NodeParam int = 7
const NodeParamList int = 8
const NodeResultList int = 9
const NodeBlock int = 10
const NodeReturn int = 11
const NodeIf int = 12
const NodeElse int = 13
const NodeFor int = 14
const NodeForClause int = 15
const NodeRangeClause int = 16
const NodeSwitch int = 17
const NodeCase int = 18
const NodeDefault int = 19
const NodeSelect int = 20
const NodeSelectCase int = 21
const NodeAssign int = 22
const NodeShortDecl int = 23
const NodeVarDecl int = 24
const NodeConstDecl int = 25
const NodeTypeDecl int = 26
const NodeStructType int = 27
const NodeInterfaceType int = 28
const NodeField int = 29
const NodeMethodSpec int = 30
const NodeBinOp int = 31
const NodeUnaryOp int = 32
const NodeCall int = 33
const NodeName int = 34
const NodeNum int = 35
const NodeStr int = 36
const NodeRuneLit int = 37
const NodeBool int = 38
const NodeNil int = 39
const NodeSelector int = 40
const NodeIndex int = 41
const NodeSliceExpr int = 42
const NodeCompositeLit int = 43
const NodeKeyValue int = 44
const NodeArrayType int = 45
const NodeSliceType int = 46
const NodeMapType int = 47
const NodePointerType int = 48
const NodeChanType int = 49
const NodeFuncType int = 50
const NodeDefer int = 51
const NodeGo int = 52
const NodeSend int = 53
const NodeRecv int = 54
const NodeIncDec int = 55
const NodeAugAssign int = 56
const NodeBreak int = 57
const NodeContinue int = 58
const NodeGoto int = 59
const NodeLabel int = 60
const NodeFallthrough int = 61
const NodeTypeAssert int = 62
const NodeVariadic int = 63
const NodeBlankIdent int = 64
const NodeIota int = 65
const NodeExprList int = 66
const NodeReceiver int = 67

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
	if nodeType == NodeFile {
		return "File"
	}
	if nodeType == NodePackage {
		return "Package"
	}
	if nodeType == NodeImport {
		return "Import"
	}
	if nodeType == NodeImportSpec {
		return "ImportSpec"
	}
	if nodeType == NodeFuncDecl {
		return "FuncDecl"
	}
	if nodeType == NodeMethodDecl {
		return "MethodDecl"
	}
	if nodeType == NodeParam {
		return "Param"
	}
	if nodeType == NodeParamList {
		return "ParamList"
	}
	if nodeType == NodeResultList {
		return "ResultList"
	}
	if nodeType == NodeBlock {
		return "Block"
	}
	if nodeType == NodeReturn {
		return "Return"
	}
	if nodeType == NodeIf {
		return "If"
	}
	if nodeType == NodeElse {
		return "Else"
	}
	if nodeType == NodeFor {
		return "For"
	}
	if nodeType == NodeForClause {
		return "ForClause"
	}
	if nodeType == NodeRangeClause {
		return "RangeClause"
	}
	if nodeType == NodeSwitch {
		return "Switch"
	}
	if nodeType == NodeCase {
		return "Case"
	}
	if nodeType == NodeDefault {
		return "Default"
	}
	if nodeType == NodeSelect {
		return "Select"
	}
	if nodeType == NodeSelectCase {
		return "SelectCase"
	}
	if nodeType == NodeAssign {
		return "Assign"
	}
	if nodeType == NodeShortDecl {
		return "ShortDecl"
	}
	if nodeType == NodeVarDecl {
		return "VarDecl"
	}
	if nodeType == NodeConstDecl {
		return "ConstDecl"
	}
	if nodeType == NodeTypeDecl {
		return "TypeDecl"
	}
	if nodeType == NodeStructType {
		return "StructType"
	}
	if nodeType == NodeInterfaceType {
		return "InterfaceType"
	}
	if nodeType == NodeField {
		return "Field"
	}
	if nodeType == NodeMethodSpec {
		return "MethodSpec"
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
	if nodeType == NodeRuneLit {
		return "RuneLit"
	}
	if nodeType == NodeBool {
		return "Bool"
	}
	if nodeType == NodeNil {
		return "Nil"
	}
	if nodeType == NodeSelector {
		return "Selector"
	}
	if nodeType == NodeIndex {
		return "Index"
	}
	if nodeType == NodeSliceExpr {
		return "SliceExpr"
	}
	if nodeType == NodeCompositeLit {
		return "CompositeLit"
	}
	if nodeType == NodeKeyValue {
		return "KeyValue"
	}
	if nodeType == NodeArrayType {
		return "ArrayType"
	}
	if nodeType == NodeSliceType {
		return "SliceType"
	}
	if nodeType == NodeMapType {
		return "MapType"
	}
	if nodeType == NodePointerType {
		return "PointerType"
	}
	if nodeType == NodeChanType {
		return "ChanType"
	}
	if nodeType == NodeFuncType {
		return "FuncType"
	}
	if nodeType == NodeDefer {
		return "Defer"
	}
	if nodeType == NodeGo {
		return "Go"
	}
	if nodeType == NodeSend {
		return "Send"
	}
	if nodeType == NodeRecv {
		return "Recv"
	}
	if nodeType == NodeIncDec {
		return "IncDec"
	}
	if nodeType == NodeAugAssign {
		return "AugAssign"
	}
	if nodeType == NodeBreak {
		return "Break"
	}
	if nodeType == NodeContinue {
		return "Continue"
	}
	if nodeType == NodeGoto {
		return "Goto"
	}
	if nodeType == NodeLabel {
		return "Label"
	}
	if nodeType == NodeFallthrough {
		return "Fallthrough"
	}
	if nodeType == NodeTypeAssert {
		return "TypeAssert"
	}
	if nodeType == NodeVariadic {
		return "Variadic"
	}
	if nodeType == NodeBlankIdent {
		return "BlankIdent"
	}
	if nodeType == NodeIota {
		return "Iota"
	}
	if nodeType == NodeExprList {
		return "ExprList"
	}
	if nodeType == NodeReceiver {
		return "Receiver"
	}
	return "Unknown"
}
