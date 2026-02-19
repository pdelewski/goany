package compiler

import "go/types"

// TokenType represents different types of tokens in the code generation
type TokenType int

const (
	// Language-specific keywords
	EmptyToken TokenType = iota
	CppKeyword
	CSharpKeyword
	RustKeyword
	JavaKeyword

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

// Token represents a single token with its type and content
type Token struct {
	Type    TokenType
	Content string
	GoType  types.Type // Go type info for shift/reduce backends, nil if N/A
	Tag     int        // Fragment tag (TagExpr, TagStmt, etc.), 0 if unset
}

// TokenTypeNames provides string representations for token types
var TokenTypeNames = map[TokenType]string{
	CppKeyword:         "CppKeyword",
	CSharpKeyword:      "CSharpKeyword",
	RustKeyword:        "RustKeyword",
	JavaKeyword:        "JavaKeyword",
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
	return t >= CppKeyword && t <= AbstractKeyword
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
			"include":   CppKeyword,
			"namespace": CppKeyword,
			"using":     CppKeyword,
			"template":  CppKeyword,
			"typename":  CppKeyword,
			"const":     CppKeyword,
			"auto":      CppKeyword,
		},
		CSharp: map[string]TokenType{
			"using":     CSharpKeyword,
			"namespace": CSharpKeyword,
			"var":       CSharpKeyword,
			"readonly":  CSharpKeyword,
			"override":  CSharpKeyword,
			"virtual":   CSharpKeyword,
			"sealed":    CSharpKeyword,
		},
		Rust: map[string]TokenType{
			"fn":    RustKeyword,
			"let":   RustKeyword,
			"mut":   RustKeyword,
			"impl":  RustKeyword,
			"trait": RustKeyword,
			"mod":   RustKeyword,
			"use":   RustKeyword,
		},
		Java: map[string]TokenType{
			"import":     JavaKeyword,
			"package":    JavaKeyword,
			"extends":    JavaKeyword,
			"implements": JavaKeyword,
			"interface":  JavaKeyword,
			"enum":       JavaKeyword,
			"throws":     JavaKeyword,
		},
	}
}
