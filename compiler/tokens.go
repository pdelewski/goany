package compiler

import (
	"go/ast"
	"go/types"
	"strings"
)

// TokenType represents different types of tokens in the code generation
type TokenType int

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
	EmptyToken TokenType = iota
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
// Attached as a pointer on Token so non-annotated tokens have zero overhead.
type OptMeta struct {
	Kind            OptKind
	VarName         string        // variable name for clone/move decisions
	ParamIndex      int           // parameter index in function signature or call
	FuncKey         string        // qualified function key ("pkg.FuncName")
	CalleeName      string        // name of function being called
	ParamName       string        // parameter name (for signature changes)
	TypeStr         string        // type string (for signature changes)
	IsInsideClosure bool          // whether this is inside a FuncLit
	NodeExpr        ast.Expr      // original Go AST expression
	NodeStmt        ast.Stmt      // original Go AST statement
	ReturnNode      *ast.ReturnStmt // for multi-return analysis
	AssignNode      *ast.AssignStmt // for move extraction analysis
	ResultIndex     int           // index in multi-return
	NumResults      int           // total number of return results
	MapContent      string        // map variable code (for &map vs map.clone())
}

// Token represents a single token with its type and content
type Token struct {
	Type     TokenType
	Content  string
	GoType   types.Type // Go type info for shift/reduce backends, nil if N/A
	Tag      int        // Fragment tag (TagExpr, TagStmt, etc.), 0 if unset
	OptMeta  *OptMeta   // nil when no optimization metadata
	Children []Token    // child tokens for tree structure; nil for leaf tokens
}

// Serialize returns the string representation of this token.
// For leaf tokens (no children), it returns Content directly.
// For tree tokens, it recursively serializes all children.
func (t Token) Serialize() string {
	if len(t.Children) == 0 {
		return t.Content
	}
	var sb strings.Builder
	for _, child := range t.Children {
		sb.WriteString(child.Serialize())
	}
	return sb.String()
}

// TokenTree builds a parent token with children and eagerly sets Content = Serialize().
func TokenTree(tokenType TokenType, tag int, children ...Token) Token {
	t := Token{
		Type:     tokenType,
		Tag:      tag,
		Children: children,
	}
	t.Content = t.Serialize()
	return t
}

// Leaf creates an atomic token with no children.
func Leaf(tokenType TokenType, content string) Token {
	return Token{
		Type:    tokenType,
		Content: content,
	}
}

// LeafTag creates an atomic token with a tag and no children.
func LeafTag(tokenType TokenType, content string, tag int) Token {
	return Token{
		Type:    tokenType,
		Content: content,
		Tag:     tag,
	}
}

// TokenTypeNames provides string representations for token types
var TokenTypeNames = map[TokenType]string{
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
}

// String returns the string representation of a TokenType
func (t TokenType) String() string {
	if name, ok := TokenTypeNames[t]; ok {
		return name
	}
	return "Unknown"
}

// CreateToken creates a new token with the given type and content
func CreateToken(tokenType TokenType, content string) Token {
	return Token{
		Type:    tokenType,
		Content: content,
	}
}

// IsKeyword returns true if the token type represents a keyword
func (t TokenType) IsKeyword() bool {
	return t == Keyword || (t >= IfKeyword && t <= AbstractKeyword)
}

// IsOperator returns true if the token type represents an operator
func (t TokenType) IsOperator() bool {
	return t >= Assignment && t <= ArithmeticOperator
}

// IsPunctuation returns true if the token type represents punctuation
func (t TokenType) IsPunctuation() bool {
	return t >= Comma && t <= RightAngle
}

// IsWhitespace returns true if the token type represents whitespace
func (t TokenType) IsWhitespace() bool {
	return t >= WhiteSpace && t <= Tab
}

// IsLiteral returns true if the token type represents a literal value
func (t TokenType) IsLiteral() bool {
	return t >= StringLiteral && t <= CharLiteral
}

// LanguageSpecificKeywords maps keywords to their target language equivalents
type LanguageKeywordMap struct {
	Cpp    map[string]TokenType
	CSharp map[string]TokenType
	Rust   map[string]TokenType
	Java   map[string]TokenType
}

// GetLanguageKeywords returns keyword mappings for different languages
func GetLanguageKeywords() LanguageKeywordMap {
	return LanguageKeywordMap{
		Cpp: map[string]TokenType{
			"include":   Keyword,
			"namespace": Keyword,
			"using":     Keyword,
			"template":  Keyword,
			"typename":  Keyword,
			"const":     Keyword,
			"auto":      Keyword,
		},
		CSharp: map[string]TokenType{
			"using":     Keyword,
			"namespace": Keyword,
			"var":       Keyword,
			"readonly":  Keyword,
			"override":  Keyword,
			"virtual":   Keyword,
			"sealed":    Keyword,
		},
		Rust: map[string]TokenType{
			"fn":    Keyword,
			"let":   Keyword,
			"mut":   Keyword,
			"impl":  Keyword,
			"trait": Keyword,
			"mod":   Keyword,
			"use":   Keyword,
		},
		Java: map[string]TokenType{
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
