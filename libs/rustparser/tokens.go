package rustparser

// Token types
const TokenEOF int = 0
const TokenIdent int = 1
const TokenNumber int = 2
const TokenString int = 3
const TokenKeyword int = 4
const TokenLParen int = 5
const TokenRParen int = 6
const TokenLBrace int = 7
const TokenRBrace int = 8
const TokenLBracket int = 9
const TokenRBracket int = 10
const TokenSemicolon int = 11
const TokenComma int = 12
const TokenColon int = 13
const TokenDot int = 14
const TokenArrow int = 15
const TokenDoubleColon int = 16
const TokenAmpersand int = 17
const TokenAssign int = 18
const TokenEq int = 19
const TokenNeq int = 20
const TokenLt int = 21
const TokenGt int = 22
const TokenLe int = 23
const TokenGe int = 24
const TokenExcl int = 25
const TokenPlus int = 26
const TokenMinus int = 27
const TokenStar int = 28
const TokenSlash int = 29
const TokenPercent int = 30
const TokenAndAnd int = 31
const TokenOrOr int = 32
const TokenHash int = 33

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

// isRustKeyword checks if a string is a Rust keyword
func isRustKeyword(s string) bool {
	if s == "fn" {
		return true
	}
	if s == "let" {
		return true
	}
	if s == "mut" {
		return true
	}
	if s == "if" {
		return true
	}
	if s == "else" {
		return true
	}
	if s == "while" {
		return true
	}
	if s == "for" {
		return true
	}
	if s == "in" {
		return true
	}
	if s == "return" {
		return true
	}
	if s == "struct" {
		return true
	}
	if s == "impl" {
		return true
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return true
	}
	if s == "pub" {
		return true
	}
	if s == "self" {
		return true
	}
	if s == "loop" {
		return true
	}
	if s == "break" {
		return true
	}
	if s == "continue" {
		return true
	}
	if s == "use" {
		return true
	}
	if s == "mod" {
		return true
	}
	if s == "const" {
		return true
	}
	if s == "type" {
		return true
	}
	if s == "enum" {
		return true
	}
	if s == "match" {
		return true
	}
	if s == "trait" {
		return true
	}
	return false
}

// TokenTypeName returns a human-readable name for token type
func TokenTypeName(tokenType int) string {
	if tokenType == TokenEOF {
		return "EOF"
	}
	if tokenType == TokenIdent {
		return "IDENT"
	}
	if tokenType == TokenNumber {
		return "NUMBER"
	}
	if tokenType == TokenString {
		return "STRING"
	}
	if tokenType == TokenKeyword {
		return "KEYWORD"
	}
	if tokenType == TokenLParen {
		return "LPAREN"
	}
	if tokenType == TokenRParen {
		return "RPAREN"
	}
	if tokenType == TokenLBrace {
		return "LBRACE"
	}
	if tokenType == TokenRBrace {
		return "RBRACE"
	}
	if tokenType == TokenLBracket {
		return "LBRACKET"
	}
	if tokenType == TokenRBracket {
		return "RBRACKET"
	}
	if tokenType == TokenSemicolon {
		return "SEMICOLON"
	}
	if tokenType == TokenComma {
		return "COMMA"
	}
	if tokenType == TokenColon {
		return "COLON"
	}
	if tokenType == TokenDot {
		return "DOT"
	}
	if tokenType == TokenArrow {
		return "ARROW"
	}
	if tokenType == TokenDoubleColon {
		return "DOUBLECOLON"
	}
	if tokenType == TokenAmpersand {
		return "AMPERSAND"
	}
	if tokenType == TokenAssign {
		return "ASSIGN"
	}
	if tokenType == TokenEq {
		return "EQ"
	}
	if tokenType == TokenNeq {
		return "NEQ"
	}
	if tokenType == TokenLt {
		return "LT"
	}
	if tokenType == TokenGt {
		return "GT"
	}
	if tokenType == TokenLe {
		return "LE"
	}
	if tokenType == TokenGe {
		return "GE"
	}
	if tokenType == TokenExcl {
		return "EXCL"
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
	if tokenType == TokenAndAnd {
		return "ANDAND"
	}
	if tokenType == TokenOrOr {
		return "OROR"
	}
	if tokenType == TokenHash {
		return "HASH"
	}
	return "UNKNOWN"
}
