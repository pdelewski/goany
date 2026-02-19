package goparser

// Token types
const TokenEOF int = 0
const TokenIdentifier int = 1
const TokenNumber int = 2
const TokenString int = 3
const TokenRune int = 4
const TokenSemicolon int = 5
const TokenColon int = 6
const TokenComma int = 7
const TokenDot int = 8
const TokenLParen int = 9
const TokenRParen int = 10
const TokenLBracket int = 11
const TokenRBracket int = 12
const TokenLBrace int = 13
const TokenRBrace int = 14
const TokenAssign int = 15
const TokenColonAssign int = 16  // :=
const TokenPlus int = 17
const TokenMinus int = 18
const TokenStar int = 19
const TokenSlash int = 20
const TokenPercent int = 21
const TokenAmpersand int = 22
const TokenPipe int = 23
const TokenCaret int = 24
const TokenLeftShift int = 25   // <<
const TokenRightShift int = 26  // >>
const TokenAmpCaret int = 27    // &^
const TokenPlusAssign int = 28  // +=
const TokenMinusAssign int = 29 // -=
const TokenStarAssign int = 30  // *=
const TokenSlashAssign int = 31 // /=
const TokenPercentAssign int = 32 // %=
const TokenAmpAssign int = 33    // &=
const TokenPipeAssign int = 34   // |=
const TokenCaretAssign int = 35  // ^=
const TokenLeftShiftAssign int = 36  // <<=
const TokenRightShiftAssign int = 37 // >>=
const TokenAmpCaretAssign int = 38   // &^=
const TokenLogicalAnd int = 39  // &&
const TokenLogicalOr int = 40   // ||
const TokenArrow int = 41       // <-
const TokenIncrement int = 42   // ++
const TokenDecrement int = 43   // --
const TokenEqual int = 44       // ==
const TokenNotEqual int = 45    // !=
const TokenLess int = 46        // <
const TokenLessEqual int = 47   // <=
const TokenGreater int = 48     // >
const TokenGreaterEqual int = 49 // >=
const TokenNot int = 50          // !
const TokenEllipsis int = 51    // ...
const TokenKeyword int = 52
const TokenTilde int = 57       // ~

// Token represents a lexical token
type Token struct {
	Type  int
	Value string
	Line  int
	Col   int
}

// NewToken creates a new token
func NewToken(tokenType int, value string, line int, col int) Token {
	return Token{
		Type:  tokenType,
		Value: value,
		Line:  line,
		Col:   col,
	}
}

// isKeyword checks if a string is a Go keyword
func isKeyword(s string) bool {
	if s == "break" {
		return true
	}
	if s == "case" {
		return true
	}
	if s == "chan" {
		return true
	}
	if s == "const" {
		return true
	}
	if s == "continue" {
		return true
	}
	if s == "default" {
		return true
	}
	if s == "defer" {
		return true
	}
	if s == "else" {
		return true
	}
	if s == "fallthrough" {
		return true
	}
	if s == "for" {
		return true
	}
	if s == "func" {
		return true
	}
	if s == "go" {
		return true
	}
	if s == "goto" {
		return true
	}
	if s == "if" {
		return true
	}
	if s == "import" {
		return true
	}
	if s == "interface" {
		return true
	}
	if s == "map" {
		return true
	}
	if s == "package" {
		return true
	}
	if s == "range" {
		return true
	}
	if s == "return" {
		return true
	}
	if s == "select" {
		return true
	}
	if s == "struct" {
		return true
	}
	if s == "switch" {
		return true
	}
	if s == "type" {
		return true
	}
	if s == "var" {
		return true
	}
	return false
}

// TokenTypeName returns a human-readable name for token type
func TokenTypeName(tokenType int) string {
	if tokenType == TokenEOF {
		return "EOF"
	}
	if tokenType == TokenIdentifier {
		return "IDENTIFIER"
	}
	if tokenType == TokenNumber {
		return "NUMBER"
	}
	if tokenType == TokenString {
		return "STRING"
	}
	if tokenType == TokenRune {
		return "RUNE"
	}
	if tokenType == TokenSemicolon {
		return "SEMICOLON"
	}
	if tokenType == TokenColon {
		return "COLON"
	}
	if tokenType == TokenComma {
		return "COMMA"
	}
	if tokenType == TokenDot {
		return "DOT"
	}
	if tokenType == TokenLParen {
		return "LPAREN"
	}
	if tokenType == TokenRParen {
		return "RPAREN"
	}
	if tokenType == TokenLBracket {
		return "LBRACKET"
	}
	if tokenType == TokenRBracket {
		return "RBRACKET"
	}
	if tokenType == TokenLBrace {
		return "LBRACE"
	}
	if tokenType == TokenRBrace {
		return "RBRACE"
	}
	if tokenType == TokenAssign {
		return "ASSIGN"
	}
	if tokenType == TokenColonAssign {
		return "COLONASSIGN"
	}
	if tokenType == TokenPlus {
		return "PLUS"
	}
	if tokenType == TokenMinus {
		return "MINUS"
	}
	if tokenType == TokenStar {
		return "STAR"
	}
	if tokenType == TokenSlash {
		return "SLASH"
	}
	if tokenType == TokenPercent {
		return "PERCENT"
	}
	if tokenType == TokenAmpersand {
		return "AMPERSAND"
	}
	if tokenType == TokenPipe {
		return "PIPE"
	}
	if tokenType == TokenCaret {
		return "CARET"
	}
	if tokenType == TokenLeftShift {
		return "LEFTSHIFT"
	}
	if tokenType == TokenRightShift {
		return "RIGHTSHIFT"
	}
	if tokenType == TokenAmpCaret {
		return "AMPCARET"
	}
	if tokenType == TokenPlusAssign {
		return "PLUSASSIGN"
	}
	if tokenType == TokenMinusAssign {
		return "MINUSASSIGN"
	}
	if tokenType == TokenStarAssign {
		return "STARASSIGN"
	}
	if tokenType == TokenSlashAssign {
		return "SLASHASSIGN"
	}
	if tokenType == TokenPercentAssign {
		return "PERCENTASSIGN"
	}
	if tokenType == TokenAmpAssign {
		return "AMPASSIGN"
	}
	if tokenType == TokenPipeAssign {
		return "PIPEASSIGN"
	}
	if tokenType == TokenCaretAssign {
		return "CARETASSIGN"
	}
	if tokenType == TokenLeftShiftAssign {
		return "LEFTSHIFTASSIGN"
	}
	if tokenType == TokenRightShiftAssign {
		return "RIGHTSHIFTASSIGN"
	}
	if tokenType == TokenAmpCaretAssign {
		return "AMPCARETASSIGN"
	}
	if tokenType == TokenLogicalAnd {
		return "LOGICALAND"
	}
	if tokenType == TokenLogicalOr {
		return "LOGICALOR"
	}
	if tokenType == TokenArrow {
		return "ARROW"
	}
	if tokenType == TokenIncrement {
		return "INCREMENT"
	}
	if tokenType == TokenDecrement {
		return "DECREMENT"
	}
	if tokenType == TokenEqual {
		return "EQUAL"
	}
	if tokenType == TokenNotEqual {
		return "NOTEQUAL"
	}
	if tokenType == TokenLess {
		return "LESS"
	}
	if tokenType == TokenLessEqual {
		return "LESSEQUAL"
	}
	if tokenType == TokenGreater {
		return "GREATER"
	}
	if tokenType == TokenGreaterEqual {
		return "GREATEREQUAL"
	}
	if tokenType == TokenNot {
		return "NOT"
	}
	if tokenType == TokenEllipsis {
		return "ELLIPSIS"
	}
	if tokenType == TokenKeyword {
		return "KEYWORD"
	}
	if tokenType == TokenTilde {
		return "TILDE"
	}
	return "UNKNOWN"
}
