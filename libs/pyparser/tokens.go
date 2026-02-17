package pyparser

// Token types
const TokenEOF int = 0
const TokenNewline int = 1
const TokenIndent int = 2
const TokenDedent int = 3
const TokenIdentifier int = 4
const TokenNumber int = 5
const TokenString int = 6
const TokenOperator int = 7
const TokenKeyword int = 8
const TokenColon int = 9
const TokenComma int = 10
const TokenLParen int = 11
const TokenRParen int = 12
const TokenLBracket int = 13
const TokenRBracket int = 14
const TokenLBrace int = 15
const TokenRBrace int = 16
const TokenDot int = 17
const TokenAssign int = 18
const TokenAt int = 19
const TokenStar int = 20
const TokenDoubleStar int = 21
const TokenArrow int = 22      // -> for type hints
const TokenPlusAssign int = 23 // +=
const TokenMinusAssign int = 24 // -=
const TokenStarAssign int = 25 // *=
const TokenSlashAssign int = 26 // /=
const TokenPercentAssign int = 27 // %=
const TokenAmpersand int = 28  // &
const TokenPipe int = 29       // |
const TokenCaret int = 30      // ^
const TokenTilde int = 31      // ~
const TokenLeftShift int = 32  // <<
const TokenRightShift int = 33 // >>
const TokenWalrus int = 34     // := (walrus operator)
const TokenColonEqual int = 35 // := alias
const TokenEllipsis int = 36   // ...

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

// Keywords list
func isKeyword(s string) bool {
	if s == "def" {
		return true
	}
	if s == "return" {
		return true
	}
	if s == "if" {
		return true
	}
	if s == "elif" {
		return true
	}
	if s == "else" {
		return true
	}
	if s == "for" {
		return true
	}
	if s == "while" {
		return true
	}
	if s == "in" {
		return true
	}
	if s == "and" {
		return true
	}
	if s == "or" {
		return true
	}
	if s == "not" {
		return true
	}
	if s == "True" {
		return true
	}
	if s == "False" {
		return true
	}
	if s == "None" {
		return true
	}
	if s == "pass" {
		return true
	}
	if s == "break" {
		return true
	}
	if s == "continue" {
		return true
	}
	if s == "print" {
		return true
	}
	if s == "range" {
		return true
	}
	if s == "import" {
		return true
	}
	if s == "from" {
		return true
	}
	if s == "as" {
		return true
	}
	if s == "lambda" {
		return true
	}
	if s == "class" {
		return true
	}
	if s == "try" {
		return true
	}
	if s == "except" {
		return true
	}
	if s == "finally" {
		return true
	}
	if s == "with" {
		return true
	}
	if s == "yield" {
		return true
	}
	if s == "raise" {
		return true
	}
	if s == "global" {
		return true
	}
	if s == "nonlocal" {
		return true
	}
	if s == "assert" {
		return true
	}
	if s == "del" {
		return true
	}
	if s == "async" {
		return true
	}
	if s == "await" {
		return true
	}
	if s == "match" {
		return true
	}
	if s == "case" {
		return true
	}
	return false
}

// TokenTypeName returns a human-readable name for token type
func TokenTypeName(tokenType int) string {
	if tokenType == TokenEOF {
		return "EOF"
	}
	if tokenType == TokenNewline {
		return "NEWLINE"
	}
	if tokenType == TokenIndent {
		return "INDENT"
	}
	if tokenType == TokenDedent {
		return "DEDENT"
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
	if tokenType == TokenOperator {
		return "OPERATOR"
	}
	if tokenType == TokenKeyword {
		return "KEYWORD"
	}
	if tokenType == TokenColon {
		return "COLON"
	}
	if tokenType == TokenComma {
		return "COMMA"
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
	if tokenType == TokenDot {
		return "DOT"
	}
	if tokenType == TokenAssign {
		return "ASSIGN"
	}
	if tokenType == TokenAt {
		return "AT"
	}
	if tokenType == TokenStar {
		return "STAR"
	}
	if tokenType == TokenDoubleStar {
		return "DOUBLESTAR"
	}
	if tokenType == TokenArrow {
		return "ARROW"
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
	if tokenType == TokenAmpersand {
		return "AMPERSAND"
	}
	if tokenType == TokenPipe {
		return "PIPE"
	}
	if tokenType == TokenCaret {
		return "CARET"
	}
	if tokenType == TokenTilde {
		return "TILDE"
	}
	if tokenType == TokenLeftShift {
		return "LEFTSHIFT"
	}
	if tokenType == TokenRightShift {
		return "RIGHTSHIFT"
	}
	if tokenType == TokenWalrus {
		return "WALRUS"
	}
	if tokenType == TokenColonEqual {
		return "COLONEQUAL"
	}
	if tokenType == TokenEllipsis {
		return "ELLIPSIS"
	}
	return "UNKNOWN"
}
