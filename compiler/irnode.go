package compiler

import (
	"go/ast"
	"go/types"
	"strings"
)

// IRNodeType represents different types of tokens in the code generation
type IRNodeType int

// NodeKind provides semantic classification for IR nodes.
type NodeKind int

const (
	KindNone    NodeKind = iota
	KindExpr             // expression
	KindStmt             // statement
	KindDecl             // declaration
	KindType             // type annotation
	KindIdent            // identifier
	KindLiteral          // literal value
)

// Language tag constants for Keyword token type
const (
	TagRust   = 100
	TagCpp    = 101
	TagCSharp = 102
	TagJava   = 103
	TagJs     = 104
)

const (
	// Language-specific keywords
	EmptyIRNode IRNodeType = iota
	Keyword              // unified keyword type; language stored in Tag field
	_                    // reserved (was CSharpKeyword)
	_                    // reserved (was RustKeyword)
	_                    // reserved (was JavaKeyword)

	// Identifiers and literals
	Identifier
	StringLiteral
	NumberLiteral
	BooleanLiteral
	CharLiteral

	// Operators
	Assignment
	BinaryOperator
	UnaryOperator
	ComparisonOperator
	LogicalOperator
	ArithmeticOperator

	// Punctuation
	Comma
	Semicolon
	Colon
	Dot

	// Parentheses and brackets
	LeftParen
	RightParen
	LeftBrace
	RightBrace
	LeftBracket
	RightBracket
	LeftAngle
	RightAngle

	// Whitespace and formatting
	WhiteSpace
	NewLine
	Tab

	// Comments
	LineComment
	BlockComment

	// Special tokens
	EOF
	Invalid

	// Control flow
	IfKeyword
	ElseKeyword
	ForKeyword
	WhileKeyword
	SwitchKeyword
	CaseKeyword
	DefaultKeyword
	BreakKeyword
	ContinueKeyword
	ReturnKeyword

	// Type-related
	TypeKeyword
	StructKeyword
	InterfaceKeyword
	ClassKeyword

	// Access modifiers
	PublicKeyword
	PrivateKeyword
	ProtectedKeyword
	StaticKeyword
	FinalKeyword
	AbstractKeyword

	// Language-specific types
	VoidType
	IntType
	StringType
	BoolType
	FloatType
	DoubleType

	// Function-related
	FunctionKeyword
	MethodKeyword
	ParameterList
	ReturnType

	// AST-level expression node types
	BinaryExpression
	UnaryExpression
	CallExpression
	SelectorExpression
	IndexExpression
	SliceExpression
	ParenExpression
	CompositeLitExpression
	FuncLitExpression
	TypeAssertExpression
	StarExpression
	KeyValueExpression
	BasicLitExpression
	IdentExpression
	EllipsisExpression
	FuncTypeExpression

	// AST-level statement node types
	AssignStatement
	BlockStatement
	ReturnStatement
	IfStatement
	ForStatement
	SwitchStatement
	CaseClauseStatement
	IncDecStatement
	ExprStatement
	BranchStatement
	DeferStatement
	GoStatement
	RangeStatement
	DeclStatement
	LabeledStatement
	SelectStatement
	SendStatement
	TypeSwitchStatement

	// AST-level declaration node types
	FuncDeclaration
	PackageDeclaration

	// AST-level type node types
	ArrayTypeNode
	MapTypeNode
	InterfaceTypeNode
	StructTypeNode
	ChanTypeNode
	ScopeNode  // auto-collected scope wrapper
	Preamble   // header/boilerplate content (includes, runtime, helpers)
)

// OptKind identifies what kind of optimizable construct a token represents.
type OptKind int

const (
	OptNone        OptKind = iota
	OptClone                 // .clone() on a call arg or return value
	OptFuncParam             // function parameter declaration
	OptCallArg               // function call argument (for ref-opt callee lookup)
	OptMapOp                 // map operation (hashMapGet, hashMapLen, etc.)
	OptReturnValue           // return value expression
	OptAssignment            // full assignment statement
)

// OptMeta carries optimization-relevant context from emitter to optimizer.
// Attached as a pointer on IRNode so non-annotated tokens have zero overhead.
type OptMeta struct {
	Kind            OptKind
	VarName         string        // variable name for clone/move decisions
	ParamIndex      int           // parameter index in function signature or call
	FuncKey         string        // qualified function key ("pkg.FuncName")
	CalleeName      string        // name of function being called
	ParamName       string        // parameter name (for signature changes)
	TypeStr         string        // type string (for signature changes)
	IsInsideClosure bool          // whether this is inside a FuncLit
	IsRefEligible   bool          // true if type is struct or slice (eligible for &T optimization)
	IsReadOnly      bool          // param determined read-only by analysis (for RefOptPass)
	IsMutRef        bool          // param determined mut-ref by analysis (for RefOptPass)
	NodeExpr        ast.Expr      // original Go AST expression
	NodeStmt        ast.Stmt      // original Go AST statement
	ReturnNode      *ast.ReturnStmt // for multi-return analysis
	AssignNode      *ast.AssignStmt // for move extraction analysis
	ResultIndex     int           // index in multi-return
	NumResults      int           // total number of return results
	MapContent      string        // map variable code (for &map vs map.clone())
}

// IRNode represents a single token with its type and content
type IRNode struct {
	Type     IRNodeType
	Kind     NodeKind   // semantic classification (KindExpr, KindStmt, etc.)
	Content  string
	GoType   types.Type // Go type info for shift/reduce backends, nil if N/A
	Tag      int        // language tag (TagRust, TagCpp, etc.) only
	OptMeta  *OptMeta   // nil when no optimization metadata
	Children []IRNode   // child tokens for tree structure; nil for leaf tokens
}

// Serialize returns the string representation of this token.
// For leaf tokens (no children), it returns Content directly.
// For tree tokens, it recursively serializes all children.
func (t IRNode) Serialize() string {
	if len(t.Children) == 0 {
		return t.Content
	}
	var sb strings.Builder
	for _, child := range t.Children {
		sb.WriteString(child.Serialize())
	}
	return sb.String()
}

// IRTree builds a parent token with children and eagerly sets Content = Serialize().
func IRTree(tokenType IRNodeType, kind NodeKind, children ...IRNode) IRNode {
	t := IRNode{
		Type:     tokenType,
		Kind:     kind,
		Children: children,
	}
	t.Content = t.Serialize()
	return t
}

// Leaf creates an atomic token with no children.
func Leaf(tokenType IRNodeType, content string) IRNode {
	return IRNode{
		Type:    tokenType,
		Content: content,
	}
}

// LeafTag creates an atomic token with a tag and no children.
func LeafTag(tokenType IRNodeType, content string, tag int) IRNode {
	return IRNode{
		Type:    tokenType,
		Content: content,
		Tag:     tag,
	}
}

// IRNodeTypeNames provides string representations for token types
var IRNodeTypeNames = map[IRNodeType]string{
	Keyword: "Keyword",
	Identifier:         "Identifier",
	StringLiteral:      "StringLiteral",
	NumberLiteral:      "NumberLiteral",
	BooleanLiteral:     "BooleanLiteral",
	CharLiteral:        "CharLiteral",
	Assignment:         "Assignment",
	BinaryOperator:     "BinaryOperator",
	UnaryOperator:      "UnaryOperator",
	ComparisonOperator: "ComparisonOperator",
	LogicalOperator:    "LogicalOperator",
	ArithmeticOperator: "ArithmeticOperator",
	Comma:              "Comma",
	Semicolon:          "Semicolon",
	Colon:              "Colon",
	Dot:                "Dot",
	LeftParen:          "LeftParen",
	RightParen:         "RightParen",
	LeftBrace:          "LeftBrace",
	RightBrace:         "RightBrace",
	LeftBracket:        "LeftBracket",
	RightBracket:       "RightBracket",
	LeftAngle:          "LeftAngle",
	RightAngle:         "RightAngle",
	WhiteSpace:         "WhiteSpace",
	NewLine:            "NewLine",
	Tab:                "Tab",
	LineComment:        "LineComment",
	BlockComment:       "BlockComment",
	EOF:                "EOF",
	Invalid:            "Invalid",
	IfKeyword:          "IfKeyword",
	ElseKeyword:        "ElseKeyword",
	ForKeyword:         "ForKeyword",
	WhileKeyword:       "WhileKeyword",
	SwitchKeyword:      "SwitchKeyword",
	CaseKeyword:        "CaseKeyword",
	DefaultKeyword:     "DefaultKeyword",
	BreakKeyword:       "BreakKeyword",
	ContinueKeyword:    "ContinueKeyword",
	ReturnKeyword:      "ReturnKeyword",
	TypeKeyword:        "TypeKeyword",
	StructKeyword:      "StructKeyword",
	InterfaceKeyword:   "InterfaceKeyword",
	ClassKeyword:       "ClassKeyword",
	PublicKeyword:      "PublicKeyword",
	PrivateKeyword:     "PrivateKeyword",
	ProtectedKeyword:   "ProtectedKeyword",
	StaticKeyword:      "StaticKeyword",
	FinalKeyword:       "FinalKeyword",
	AbstractKeyword:    "AbstractKeyword",
	VoidType:           "VoidType",
	IntType:            "IntType",
	StringType:         "StringType",
	BoolType:           "BoolType",
	FloatType:          "FloatType",
	DoubleType:         "DoubleType",
	FunctionKeyword:    "FunctionKeyword",
	MethodKeyword:      "MethodKeyword",
	ParameterList:      "ParameterList",
	ReturnType:         "ReturnType",

	// AST-level expression node types
	BinaryExpression:       "BinaryExpression",
	UnaryExpression:        "UnaryExpression",
	CallExpression:         "CallExpression",
	SelectorExpression:     "SelectorExpression",
	IndexExpression:        "IndexExpression",
	SliceExpression:        "SliceExpression",
	ParenExpression:        "ParenExpression",
	CompositeLitExpression: "CompositeLitExpression",
	FuncLitExpression:      "FuncLitExpression",
	TypeAssertExpression:   "TypeAssertExpression",
	StarExpression:         "StarExpression",
	KeyValueExpression:     "KeyValueExpression",
	BasicLitExpression:     "BasicLitExpression",
	IdentExpression:        "IdentExpression",
	EllipsisExpression:     "EllipsisExpression",
	FuncTypeExpression:     "FuncTypeExpression",

	// AST-level statement node types
	AssignStatement:     "AssignStatement",
	BlockStatement:      "BlockStatement",
	ReturnStatement:     "ReturnStatement",
	IfStatement:         "IfStatement",
	ForStatement:        "ForStatement",
	SwitchStatement:     "SwitchStatement",
	CaseClauseStatement: "CaseClauseStatement",
	IncDecStatement:     "IncDecStatement",
	ExprStatement:       "ExprStatement",
	BranchStatement:     "BranchStatement",
	DeferStatement:      "DeferStatement",
	GoStatement:         "GoStatement",
	RangeStatement:      "RangeStatement",
	DeclStatement:       "DeclStatement",
	LabeledStatement:    "LabeledStatement",
	SelectStatement:     "SelectStatement",
	SendStatement:       "SendStatement",
	TypeSwitchStatement: "TypeSwitchStatement",

	// AST-level declaration node types
	FuncDeclaration:    "FuncDeclaration",
	PackageDeclaration: "PackageDeclaration",

	// AST-level type node types
	ArrayTypeNode:     "ArrayTypeNode",
	MapTypeNode:       "MapTypeNode",
	InterfaceTypeNode: "InterfaceTypeNode",
	StructTypeNode:    "StructTypeNode",
	ChanTypeNode:      "ChanTypeNode",
	ScopeNode:         "ScopeNode",
	Preamble:          "Preamble",
}

// String returns the string representation of a IRNodeType
func (t IRNodeType) String() string {
	if name, ok := IRNodeTypeNames[t]; ok {
		return name
	}
	return "Unknown"
}

// CreateIRNode creates a new token with the given type and content
func CreateIRNode(tokenType IRNodeType, content string) IRNode {
	return IRNode{
		Type:    tokenType,
		Content: content,
	}
}

// IsKeyword returns true if the token type represents a keyword
func (t IRNodeType) IsKeyword() bool {
	return t == Keyword || (t >= IfKeyword && t <= AbstractKeyword)
}

// IsOperator returns true if the token type represents an operator
func (t IRNodeType) IsOperator() bool {
	return t >= Assignment && t <= ArithmeticOperator
}

// IsPunctuation returns true if the token type represents punctuation
func (t IRNodeType) IsPunctuation() bool {
	return t >= Comma && t <= RightAngle
}

// IsWhitespace returns true if the token type represents whitespace
func (t IRNodeType) IsWhitespace() bool {
	return t >= WhiteSpace && t <= Tab
}

// IsLiteral returns true if the token type represents a literal value
func (t IRNodeType) IsLiteral() bool {
	return t >= StringLiteral && t <= CharLiteral
}

// LanguageSpecificKeywords maps keywords to their target language equivalents
type LanguageKeywordMap struct {
	Cpp    map[string]IRNodeType
	CSharp map[string]IRNodeType
	Rust   map[string]IRNodeType
	Java   map[string]IRNodeType
}

// GetLanguageKeywords returns keyword mappings for different languages
func GetLanguageKeywords() LanguageKeywordMap {
	return LanguageKeywordMap{
		Cpp: map[string]IRNodeType{
			"include":   Keyword,
			"namespace": Keyword,
			"using":     Keyword,
			"template":  Keyword,
			"typename":  Keyword,
			"const":     Keyword,
			"auto":      Keyword,
		},
		CSharp: map[string]IRNodeType{
			"using":     Keyword,
			"namespace": Keyword,
			"var":       Keyword,
			"readonly":  Keyword,
			"override":  Keyword,
			"virtual":   Keyword,
			"sealed":    Keyword,
		},
		Rust: map[string]IRNodeType{
			"fn":    Keyword,
			"let":   Keyword,
			"mut":   Keyword,
			"impl":  Keyword,
			"trait": Keyword,
			"mod":   Keyword,
			"use":   Keyword,
		},
		Java: map[string]IRNodeType{
			"import":     Keyword,
			"package":    Keyword,
			"extends":    Keyword,
			"implements": Keyword,
			"interface":  Keyword,
			"enum":       Keyword,
			"throws":     Keyword,
		},
	}
}
