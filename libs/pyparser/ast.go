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
const NodeDict int = 25
const NodeDictEntry int = 26
const NodeImport int = 27
const NodeImportFrom int = 28
const NodeLambda int = 29
const NodeStarArg int = 30
const NodeKwArg int = 31
const NodeDecorator int = 32
const NodeClass int = 33
const NodeTry int = 34
const NodeExcept int = 35
const NodeFinally int = 36
const NodeWith int = 37
const NodeYield int = 38
const NodeListComp int = 39
const NodeTypeHint int = 40
const NodeRaise int = 41
const NodeAugAssign int = 42   // Augmented assignment (+=, -=, etc.)
const NodeSlice int = 43       // Slice expression
const NodeTuple int = 44       // Tuple literal/unpacking
const NodeSet int = 45         // Set literal
const NodeSetComp int = 46     // Set comprehension
const NodeDictComp int = 47    // Dict comprehension
const NodeGeneratorExp int = 48 // Generator expression
const NodeAssert int = 49      // Assert statement
const NodeGlobal int = 50      // Global statement
const NodeNonlocal int = 51    // Nonlocal statement
const NodeDelete int = 52      // Del statement
const NodeAsyncFunctionDef int = 53 // Async function definition
const NodeAwait int = 54       // Await expression
const NodeNamedExpr int = 55   // Named expression (walrus operator)
const NodeFString int = 56     // f-string
const NodeFormattedValue int = 57 // Formatted value in f-string
const NodeAnnotatedAssign int = 58 // Annotated assignment (x: int = 1)

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
	if nodeType == NodeDict {
		return "Dict"
	}
	if nodeType == NodeDictEntry {
		return "DictEntry"
	}
	if nodeType == NodeImport {
		return "Import"
	}
	if nodeType == NodeImportFrom {
		return "ImportFrom"
	}
	if nodeType == NodeLambda {
		return "Lambda"
	}
	if nodeType == NodeStarArg {
		return "StarArg"
	}
	if nodeType == NodeKwArg {
		return "KwArg"
	}
	if nodeType == NodeDecorator {
		return "Decorator"
	}
	if nodeType == NodeClass {
		return "Class"
	}
	if nodeType == NodeTry {
		return "Try"
	}
	if nodeType == NodeExcept {
		return "Except"
	}
	if nodeType == NodeFinally {
		return "Finally"
	}
	if nodeType == NodeWith {
		return "With"
	}
	if nodeType == NodeYield {
		return "Yield"
	}
	if nodeType == NodeListComp {
		return "ListComp"
	}
	if nodeType == NodeTypeHint {
		return "TypeHint"
	}
	if nodeType == NodeRaise {
		return "Raise"
	}
	if nodeType == NodeAugAssign {
		return "AugAssign"
	}
	if nodeType == NodeSlice {
		return "Slice"
	}
	if nodeType == NodeTuple {
		return "Tuple"
	}
	if nodeType == NodeSet {
		return "Set"
	}
	if nodeType == NodeSetComp {
		return "SetComp"
	}
	if nodeType == NodeDictComp {
		return "DictComp"
	}
	if nodeType == NodeGeneratorExp {
		return "GeneratorExp"
	}
	if nodeType == NodeAssert {
		return "Assert"
	}
	if nodeType == NodeGlobal {
		return "Global"
	}
	if nodeType == NodeNonlocal {
		return "Nonlocal"
	}
	if nodeType == NodeDelete {
		return "Delete"
	}
	if nodeType == NodeAsyncFunctionDef {
		return "AsyncFunctionDef"
	}
	if nodeType == NodeAwait {
		return "Await"
	}
	if nodeType == NodeNamedExpr {
		return "NamedExpr"
	}
	if nodeType == NodeFString {
		return "FString"
	}
	if nodeType == NodeFormattedValue {
		return "FormattedValue"
	}
	if nodeType == NodeAnnotatedAssign {
		return "AnnotatedAssign"
	}
	return "Unknown"
}
