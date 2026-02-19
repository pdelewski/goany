package goparser

// Character classification functions

func isLetter(ch int) bool {
	if ch >= int('a') && ch <= int('z') {
		return true
	}
	if ch >= int('A') && ch <= int('Z') {
		return true
	}
	if ch == int('_') {
		return true
	}
	return false
}

func isDigit(ch int) bool {
	return ch >= int('0') && ch <= int('9')
}

func isAlphaNumeric(ch int) bool {
	return isLetter(ch) || isDigit(ch)
}

func isWhitespace(ch int) bool {
	return ch == int(' ') || ch == int('\t') || ch == int('\r')
}

func isHexDigit(ch int) bool {
	if ch >= int('0') && ch <= int('9') {
		return true
	}
	if ch >= int('a') && ch <= int('f') {
		return true
	}
	if ch >= int('A') && ch <= int('F') {
		return true
	}
	return false
}

func isOctalDigit(ch int) bool {
	return ch >= int('0') && ch <= int('7')
}

func isBinaryDigit(ch int) bool {
	return ch == int('0') || ch == int('1')
}

func charToString(ch int) string {
	// Lookup table for printable ASCII characters
	if ch == 32 {
		return " "
	} else if ch == 33 {
		return "!"
	} else if ch == 34 {
		return "\""
	} else if ch == 35 {
		return "#"
	} else if ch == 36 {
		return "$"
	} else if ch == 37 {
		return "%"
	} else if ch == 38 {
		return "&"
	} else if ch == 39 {
		return "'"
	} else if ch == 40 {
		return "("
	} else if ch == 41 {
		return ")"
	} else if ch == 42 {
		return "*"
	} else if ch == 43 {
		return "+"
	} else if ch == 44 {
		return ","
	} else if ch == 45 {
		return "-"
	} else if ch == 46 {
		return "."
	} else if ch == 47 {
		return "/"
	} else if ch == 48 {
		return "0"
	} else if ch == 49 {
		return "1"
	} else if ch == 50 {
		return "2"
	} else if ch == 51 {
		return "3"
	} else if ch == 52 {
		return "4"
	} else if ch == 53 {
		return "5"
	} else if ch == 54 {
		return "6"
	} else if ch == 55 {
		return "7"
	} else if ch == 56 {
		return "8"
	} else if ch == 57 {
		return "9"
	} else if ch == 58 {
		return ":"
	} else if ch == 59 {
		return ";"
	} else if ch == 60 {
		return "<"
	} else if ch == 61 {
		return "="
	} else if ch == 62 {
		return ">"
	} else if ch == 63 {
		return "?"
	} else if ch == 64 {
		return "@"
	} else if ch == 65 {
		return "A"
	} else if ch == 66 {
		return "B"
	} else if ch == 67 {
		return "C"
	} else if ch == 68 {
		return "D"
	} else if ch == 69 {
		return "E"
	} else if ch == 70 {
		return "F"
	} else if ch == 71 {
		return "G"
	} else if ch == 72 {
		return "H"
	} else if ch == 73 {
		return "I"
	} else if ch == 74 {
		return "J"
	} else if ch == 75 {
		return "K"
	} else if ch == 76 {
		return "L"
	} else if ch == 77 {
		return "M"
	} else if ch == 78 {
		return "N"
	} else if ch == 79 {
		return "O"
	} else if ch == 80 {
		return "P"
	} else if ch == 81 {
		return "Q"
	} else if ch == 82 {
		return "R"
	} else if ch == 83 {
		return "S"
	} else if ch == 84 {
		return "T"
	} else if ch == 85 {
		return "U"
	} else if ch == 86 {
		return "V"
	} else if ch == 87 {
		return "W"
	} else if ch == 88 {
		return "X"
	} else if ch == 89 {
		return "Y"
	} else if ch == 90 {
		return "Z"
	} else if ch == 91 {
		return "["
	} else if ch == 92 {
		return "\\"
	} else if ch == 93 {
		return "]"
	} else if ch == 94 {
		return "^"
	} else if ch == 95 {
		return "_"
	} else if ch == 96 {
		return "`"
	} else if ch == 97 {
		return "a"
	} else if ch == 98 {
		return "b"
	} else if ch == 99 {
		return "c"
	} else if ch == 100 {
		return "d"
	} else if ch == 101 {
		return "e"
	} else if ch == 102 {
		return "f"
	} else if ch == 103 {
		return "g"
	} else if ch == 104 {
		return "h"
	} else if ch == 105 {
		return "i"
	} else if ch == 106 {
		return "j"
	} else if ch == 107 {
		return "k"
	} else if ch == 108 {
		return "l"
	} else if ch == 109 {
		return "m"
	} else if ch == 110 {
		return "n"
	} else if ch == 111 {
		return "o"
	} else if ch == 112 {
		return "p"
	} else if ch == 113 {
		return "q"
	} else if ch == 114 {
		return "r"
	} else if ch == 115 {
		return "s"
	} else if ch == 116 {
		return "t"
	} else if ch == 117 {
		return "u"
	} else if ch == 118 {
		return "v"
	} else if ch == 119 {
		return "w"
	} else if ch == 120 {
		return "x"
	} else if ch == 121 {
		return "y"
	} else if ch == 122 {
		return "z"
	} else if ch == 123 {
		return "{"
	} else if ch == 124 {
		return "|"
	} else if ch == 125 {
		return "}"
	} else if ch == 126 {
		return "~"
	}
	return ""
}

// needsSemicolon returns true if the given token type should trigger
// semicolon insertion when followed by a newline (Go spec rules)
func needsSemicolon(tokenType int) bool {
	if tokenType == TokenIdentifier {
		return true
	}
	if tokenType == TokenNumber {
		return true
	}
	if tokenType == TokenString {
		return true
	}
	if tokenType == TokenRune {
		return true
	}
	if tokenType == TokenRParen {
		return true
	}
	if tokenType == TokenRBracket {
		return true
	}
	if tokenType == TokenRBrace {
		return true
	}
	if tokenType == TokenIncrement {
		return true
	}
	if tokenType == TokenDecrement {
		return true
	}
	if tokenType == TokenKeyword {
		return true
	}
	return false
}

// needsSemicolonKeyword checks if a keyword value triggers semicolon insertion
func needsSemicolonKeyword(value string) bool {
	if value == "break" {
		return true
	}
	if value == "continue" {
		return true
	}
	if value == "fallthrough" {
		return true
	}
	if value == "return" {
		return true
	}
	return false
}

// tokenizeString handles Go interpreted string literals ("...")
func tokenizeString(input string, pos int, col int, line int) (Token, int, int, int) {
	strStartCol := col
	strStartLine := line
	// Skip opening "
	pos = pos + 1
	col = col + 1

	value := ""

	for pos < len(input) {
		ch := int(input[pos])

		if ch == int('"') {
			pos = pos + 1
			col = col + 1
			break
		}

		if ch == int('\n') {
			// Unterminated string
			break
		}

		// Handle escape sequences
		if ch == int('\\') && pos+1 < len(input) {
			pos = pos + 1
			col = col + 1
			escaped := int(input[pos])
			if escaped == int('n') {
				value = value + "\\n"
			} else if escaped == int('t') {
				value = value + "\\t"
			} else if escaped == int('r') {
				value = value + "\\r"
			} else if escaped == int('\\') {
				value = value + "\\\\"
			} else if escaped == int('"') {
				value = value + "\\\""
			} else if escaped == int('\'') {
				value = value + "\\'"
			} else if escaped == int('0') {
				value = value + "\\0"
			} else if escaped == int('a') {
				value = value + "\\a"
			} else if escaped == int('b') {
				value = value + "\\b"
			} else if escaped == int('f') {
				value = value + "\\f"
			} else if escaped == int('v') {
				value = value + "\\v"
			} else if escaped == int('x') {
				value = value + "\\x"
				pos = pos + 1
				col = col + 1
				// Read two hex digits
				for i := 0; i < 2 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				continue
			} else if escaped == int('u') {
				value = value + "\\u"
				pos = pos + 1
				col = col + 1
				for i := 0; i < 4 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				continue
			} else if escaped == int('U') {
				value = value + "\\U"
				pos = pos + 1
				col = col + 1
				for i := 0; i < 8 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				continue
			} else {
				value = value + charToString(escaped)
			}
			pos = pos + 1
			col = col + 1
			continue
		}

		value = value + charToString(ch)
		pos = pos + 1
		col = col + 1
	}

	return NewToken(TokenString, value, strStartLine, strStartCol), pos, col, line
}

// tokenizeRawString handles Go raw string literals (`...`)
func tokenizeRawString(input string, pos int, col int, line int) (Token, int, int, int) {
	strStartCol := col
	strStartLine := line
	// Skip opening `
	pos = pos + 1
	col = col + 1

	value := ""

	for pos < len(input) {
		ch := int(input[pos])

		if ch == int('`') {
			pos = pos + 1
			col = col + 1
			break
		}

		if ch == int('\n') {
			value = value + "\\n"
			pos = pos + 1
			line = line + 1
			col = 1
			continue
		}

		value = value + charToString(ch)
		pos = pos + 1
		col = col + 1
	}

	return NewToken(TokenString, value, strStartLine, strStartCol), pos, col, line
}

// tokenizeRune handles Go rune literals ('...')
func tokenizeRune(input string, pos int, col int, line int) (Token, int, int, int) {
	startCol := col
	// Skip opening '
	pos = pos + 1
	col = col + 1

	value := ""

	if pos < len(input) {
		ch := int(input[pos])
		if ch == int('\\') && pos+1 < len(input) {
			// Escape sequence
			pos = pos + 1
			col = col + 1
			escaped := int(input[pos])
			if escaped == int('n') {
				value = "\\n"
			} else if escaped == int('t') {
				value = "\\t"
			} else if escaped == int('r') {
				value = "\\r"
			} else if escaped == int('\\') {
				value = "\\\\"
			} else if escaped == int('\'') {
				value = "\\'"
			} else if escaped == int('0') {
				value = "\\0"
			} else if escaped == int('a') {
				value = "\\a"
			} else if escaped == int('x') {
				value = "\\x"
				pos = pos + 1
				col = col + 1
				for i := 0; i < 2 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				// Skip closing '
				if pos < len(input) && int(input[pos]) == int('\'') {
					pos = pos + 1
					col = col + 1
				}
				return NewToken(TokenRune, value, line, startCol), pos, col, line
			} else if escaped == int('u') {
				value = "\\u"
				pos = pos + 1
				col = col + 1
				for i := 0; i < 4 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				if pos < len(input) && int(input[pos]) == int('\'') {
					pos = pos + 1
					col = col + 1
				}
				return NewToken(TokenRune, value, line, startCol), pos, col, line
			} else if escaped == int('U') {
				value = "\\U"
				pos = pos + 1
				col = col + 1
				for i := 0; i < 8 && pos < len(input) && isHexDigit(int(input[pos])); i++ {
					value = value + charToString(int(input[pos]))
					pos = pos + 1
					col = col + 1
				}
				if pos < len(input) && int(input[pos]) == int('\'') {
					pos = pos + 1
					col = col + 1
				}
				return NewToken(TokenRune, value, line, startCol), pos, col, line
			} else {
				value = charToString(escaped)
			}
			pos = pos + 1
			col = col + 1
		} else {
			value = charToString(ch)
			pos = pos + 1
			col = col + 1
		}
	}

	// Skip closing '
	if pos < len(input) && int(input[pos]) == int('\'') {
		pos = pos + 1
		col = col + 1
	}

	return NewToken(TokenRune, value, line, startCol), pos, col, line
}

// Tokenize converts Go source code into a slice of tokens
func Tokenize(input string) []Token {
	var tokens []Token

	pos := 0
	line := 1
	col := 1
	lastTokenType := TokenEOF
	lastKeywordValue := ""

	for pos < len(input) {
		ch := int(input[pos])

		// Handle newlines - semicolon insertion
		if ch == int('\n') {
			// Go semicolon insertion rule
			insertSemi := false
			if lastTokenType == TokenKeyword {
				insertSemi = needsSemicolonKeyword(lastKeywordValue)
			} else {
				insertSemi = needsSemicolon(lastTokenType)
			}
			if insertSemi {
				tokens = append(tokens, NewToken(TokenSemicolon, ";", line, col))
				lastTokenType = TokenSemicolon
				lastKeywordValue = ""
			}
			pos = pos + 1
			line = line + 1
			col = 1
			continue
		}

		// Skip whitespace
		if isWhitespace(ch) {
			pos = pos + 1
			col = col + 1
			continue
		}

		// Skip single-line comments
		if ch == int('/') && pos+1 < len(input) && int(input[pos+1]) == int('/') {
			for pos < len(input) && int(input[pos]) != int('\n') {
				pos = pos + 1
			}
			continue
		}

		// Skip multi-line comments
		if ch == int('/') && pos+1 < len(input) && int(input[pos+1]) == int('*') {
			pos = pos + 2
			col = col + 2
			for pos+1 < len(input) {
				if int(input[pos]) == int('*') && int(input[pos+1]) == int('/') {
					pos = pos + 2
					col = col + 2
					break
				}
				if int(input[pos]) == int('\n') {
					line = line + 1
					col = 1
				} else {
					col = col + 1
				}
				pos = pos + 1
			}
			continue
		}

		// Identifiers and keywords
		if isLetter(ch) {
			identStartCol := col
			value := ""
			for pos < len(input) && isAlphaNumeric(int(input[pos])) {
				value = value + charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}

			// Check for special identifier values
			if value == "true" || value == "false" {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
				lastTokenType = TokenIdentifier
				lastKeywordValue = ""
				continue
			}
			if value == "nil" {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
				lastTokenType = TokenIdentifier
				lastKeywordValue = ""
				continue
			}
			if value == "iota" {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
				lastTokenType = TokenIdentifier
				lastKeywordValue = ""
				continue
			}

			if isKeyword(value) {
				tokens = append(tokens, NewToken(TokenKeyword, value, line, identStartCol))
				lastTokenType = TokenKeyword
				lastKeywordValue = value
			} else {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
				lastTokenType = TokenIdentifier
				lastKeywordValue = ""
			}
			continue
		}

		// Numbers
		if isDigit(ch) {
			numStartCol := col
			value := ""

			// Check for hex, octal, binary prefixes
			if ch == int('0') && pos+1 < len(input) {
				nextCh := int(input[pos+1])
				if nextCh == int('x') || nextCh == int('X') {
					value = "0x"
					pos = pos + 2
					col = col + 2
					for pos < len(input) && (isHexDigit(int(input[pos])) || int(input[pos]) == int('_')) {
						if int(input[pos]) != int('_') {
							value = value + charToString(int(input[pos]))
						}
						pos = pos + 1
						col = col + 1
					}
					tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
					lastTokenType = TokenNumber
					lastKeywordValue = ""
					continue
				} else if nextCh == int('o') || nextCh == int('O') {
					value = "0o"
					pos = pos + 2
					col = col + 2
					for pos < len(input) && (isOctalDigit(int(input[pos])) || int(input[pos]) == int('_')) {
						if int(input[pos]) != int('_') {
							value = value + charToString(int(input[pos]))
						}
						pos = pos + 1
						col = col + 1
					}
					tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
					lastTokenType = TokenNumber
					lastKeywordValue = ""
					continue
				} else if nextCh == int('b') || nextCh == int('B') {
					value = "0b"
					pos = pos + 2
					col = col + 2
					for pos < len(input) && (isBinaryDigit(int(input[pos])) || int(input[pos]) == int('_')) {
						if int(input[pos]) != int('_') {
							value = value + charToString(int(input[pos]))
						}
						pos = pos + 1
						col = col + 1
					}
					tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
					lastTokenType = TokenNumber
					lastKeywordValue = ""
					continue
				}
			}

			// Regular decimal number (with possible decimal point and exponent)
			for pos < len(input) {
				currCh := int(input[pos])
				if isDigit(currCh) || currCh == int('_') {
					if currCh != int('_') {
						value = value + charToString(currCh)
					}
					pos = pos + 1
					col = col + 1
				} else if currCh == int('.') {
					// Check if next char is a digit (otherwise it's a method call or selector)
					if pos+1 < len(input) && isDigit(int(input[pos+1])) {
						value = value + "."
						pos = pos + 1
						col = col + 1
					} else {
						break
					}
				} else if currCh == int('e') || currCh == int('E') {
					value = value + charToString(currCh)
					pos = pos + 1
					col = col + 1
					if pos < len(input) && (int(input[pos]) == int('+') || int(input[pos]) == int('-')) {
						value = value + charToString(int(input[pos]))
						pos = pos + 1
						col = col + 1
					}
				} else {
					break
				}
			}
			tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
			lastTokenType = TokenNumber
			lastKeywordValue = ""
			continue
		}

		// String literals
		if ch == int('"') {
			var strToken Token
			strToken, pos, col, line = tokenizeString(input, pos, col, line)
			tokens = append(tokens, strToken)
			lastTokenType = TokenString
			lastKeywordValue = ""
			continue
		}

		// Raw string literals
		if ch == int('`') {
			var strToken Token
			strToken, pos, col, line = tokenizeRawString(input, pos, col, line)
			tokens = append(tokens, strToken)
			lastTokenType = TokenString
			lastKeywordValue = ""
			continue
		}

		// Rune literals
		if ch == int('\'') {
			var runeToken Token
			runeToken, pos, col, line = tokenizeRune(input, pos, col, line)
			tokens = append(tokens, runeToken)
			lastTokenType = TokenRune
			lastKeywordValue = ""
			continue
		}

		// Three-character operators
		if pos+2 < len(input) {
			c1 := int(input[pos])
			c2 := int(input[pos+1])
			c3 := int(input[pos+2])
			if c1 == int('.') && c2 == int('.') && c3 == int('.') {
				tokens = append(tokens, NewToken(TokenEllipsis, "...", line, col))
				lastTokenType = TokenEllipsis
				lastKeywordValue = ""
				pos = pos + 3
				col = col + 3
				continue
			}
			if c1 == int('<') && c2 == int('<') && c3 == int('=') {
				tokens = append(tokens, NewToken(TokenLeftShiftAssign, "<<=", line, col))
				lastTokenType = TokenLeftShiftAssign
				lastKeywordValue = ""
				pos = pos + 3
				col = col + 3
				continue
			}
			if c1 == int('>') && c2 == int('>') && c3 == int('=') {
				tokens = append(tokens, NewToken(TokenRightShiftAssign, ">>=", line, col))
				lastTokenType = TokenRightShiftAssign
				lastKeywordValue = ""
				pos = pos + 3
				col = col + 3
				continue
			}
			if c1 == int('&') && c2 == int('^') && c3 == int('=') {
				tokens = append(tokens, NewToken(TokenAmpCaretAssign, "&^=", line, col))
				lastTokenType = TokenAmpCaretAssign
				lastKeywordValue = ""
				pos = pos + 3
				col = col + 3
				continue
			}
		}

		// Two-character operators
		if pos+1 < len(input) {
			c1 := int(input[pos])
			c2 := int(input[pos+1])
			if c1 == int(':') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenColonAssign, ":=", line, col))
				lastTokenType = TokenColonAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('=') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenEqual, "==", line, col))
				lastTokenType = TokenEqual
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('!') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenNotEqual, "!=", line, col))
				lastTokenType = TokenNotEqual
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('<') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenLessEqual, "<=", line, col))
				lastTokenType = TokenLessEqual
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('>') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenGreaterEqual, ">=", line, col))
				lastTokenType = TokenGreaterEqual
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('<') && c2 == int('<') {
				tokens = append(tokens, NewToken(TokenLeftShift, "<<", line, col))
				lastTokenType = TokenLeftShift
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('>') && c2 == int('>') {
				tokens = append(tokens, NewToken(TokenRightShift, ">>", line, col))
				lastTokenType = TokenRightShift
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('&') && c2 == int('^') {
				tokens = append(tokens, NewToken(TokenAmpCaret, "&^", line, col))
				lastTokenType = TokenAmpCaret
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('&') && c2 == int('&') {
				tokens = append(tokens, NewToken(TokenLogicalAnd, "&&", line, col))
				lastTokenType = TokenLogicalAnd
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('|') && c2 == int('|') {
				tokens = append(tokens, NewToken(TokenLogicalOr, "||", line, col))
				lastTokenType = TokenLogicalOr
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('<') && c2 == int('-') {
				tokens = append(tokens, NewToken(TokenArrow, "<-", line, col))
				lastTokenType = TokenArrow
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('+') && c2 == int('+') {
				tokens = append(tokens, NewToken(TokenIncrement, "++", line, col))
				lastTokenType = TokenIncrement
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('-') && c2 == int('-') {
				tokens = append(tokens, NewToken(TokenDecrement, "--", line, col))
				lastTokenType = TokenDecrement
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('+') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenPlusAssign, "+=", line, col))
				lastTokenType = TokenPlusAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('-') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenMinusAssign, "-=", line, col))
				lastTokenType = TokenMinusAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('*') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenStarAssign, "*=", line, col))
				lastTokenType = TokenStarAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('/') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenSlashAssign, "/=", line, col))
				lastTokenType = TokenSlashAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('%') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenPercentAssign, "%=", line, col))
				lastTokenType = TokenPercentAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('&') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenAmpAssign, "&=", line, col))
				lastTokenType = TokenAmpAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('|') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenPipeAssign, "|=", line, col))
				lastTokenType = TokenPipeAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
			if c1 == int('^') && c2 == int('=') {
				tokens = append(tokens, NewToken(TokenCaretAssign, "^=", line, col))
				lastTokenType = TokenCaretAssign
				lastKeywordValue = ""
				pos = pos + 2
				col = col + 2
				continue
			}
		}

		// Single-character tokens
		charStartCol := col
		if ch == int(';') {
			tokens = append(tokens, NewToken(TokenSemicolon, ";", line, charStartCol))
			lastTokenType = TokenSemicolon
		} else if ch == int(':') {
			tokens = append(tokens, NewToken(TokenColon, ":", line, charStartCol))
			lastTokenType = TokenColon
		} else if ch == int(',') {
			tokens = append(tokens, NewToken(TokenComma, ",", line, charStartCol))
			lastTokenType = TokenComma
		} else if ch == int('.') {
			tokens = append(tokens, NewToken(TokenDot, ".", line, charStartCol))
			lastTokenType = TokenDot
		} else if ch == int('(') {
			tokens = append(tokens, NewToken(TokenLParen, "(", line, charStartCol))
			lastTokenType = TokenLParen
		} else if ch == int(')') {
			tokens = append(tokens, NewToken(TokenRParen, ")", line, charStartCol))
			lastTokenType = TokenRParen
		} else if ch == int('[') {
			tokens = append(tokens, NewToken(TokenLBracket, "[", line, charStartCol))
			lastTokenType = TokenLBracket
		} else if ch == int(']') {
			tokens = append(tokens, NewToken(TokenRBracket, "]", line, charStartCol))
			lastTokenType = TokenRBracket
		} else if ch == int('{') {
			tokens = append(tokens, NewToken(TokenLBrace, "{", line, charStartCol))
			lastTokenType = TokenLBrace
		} else if ch == int('}') {
			tokens = append(tokens, NewToken(TokenRBrace, "}", line, charStartCol))
			lastTokenType = TokenRBrace
		} else if ch == int('=') {
			tokens = append(tokens, NewToken(TokenAssign, "=", line, charStartCol))
			lastTokenType = TokenAssign
		} else if ch == int('+') {
			tokens = append(tokens, NewToken(TokenPlus, "+", line, charStartCol))
			lastTokenType = TokenPlus
		} else if ch == int('-') {
			tokens = append(tokens, NewToken(TokenMinus, "-", line, charStartCol))
			lastTokenType = TokenMinus
		} else if ch == int('*') {
			tokens = append(tokens, NewToken(TokenStar, "*", line, charStartCol))
			lastTokenType = TokenStar
		} else if ch == int('/') {
			tokens = append(tokens, NewToken(TokenSlash, "/", line, charStartCol))
			lastTokenType = TokenSlash
		} else if ch == int('%') {
			tokens = append(tokens, NewToken(TokenPercent, "%", line, charStartCol))
			lastTokenType = TokenPercent
		} else if ch == int('&') {
			tokens = append(tokens, NewToken(TokenAmpersand, "&", line, charStartCol))
			lastTokenType = TokenAmpersand
		} else if ch == int('|') {
			tokens = append(tokens, NewToken(TokenPipe, "|", line, charStartCol))
			lastTokenType = TokenPipe
		} else if ch == int('^') {
			tokens = append(tokens, NewToken(TokenCaret, "^", line, charStartCol))
			lastTokenType = TokenCaret
		} else if ch == int('<') {
			tokens = append(tokens, NewToken(TokenLess, "<", line, charStartCol))
			lastTokenType = TokenLess
		} else if ch == int('>') {
			tokens = append(tokens, NewToken(TokenGreater, ">", line, charStartCol))
			lastTokenType = TokenGreater
		} else if ch == int('!') {
			tokens = append(tokens, NewToken(TokenNot, "!", line, charStartCol))
			lastTokenType = TokenNot
		}
		lastKeywordValue = ""
		pos = pos + 1
		col = col + 1
	}

	// Insert final semicolon if needed
	finalInsertSemi := false
	if lastTokenType == TokenKeyword {
		finalInsertSemi = needsSemicolonKeyword(lastKeywordValue)
	} else {
		finalInsertSemi = needsSemicolon(lastTokenType)
	}
	if finalInsertSemi {
		tokens = append(tokens, NewToken(TokenSemicolon, ";", line, col))
	}

	// Add EOF token
	tokens = append(tokens, NewToken(TokenEOF, "", line, col))

	return tokens
}

// Helper functions for token access

func peekToken(tokens []Token, pos int) Token {
	if pos >= len(tokens) {
		return NewToken(TokenEOF, "", 0, 0)
	}
	return tokens[pos]
}

func peekTokenType(tokens []Token, pos int) int {
	if pos >= len(tokens) {
		return TokenEOF
	}
	return tokens[pos].Type
}

func peekTokenValue(tokens []Token, pos int) string {
	if pos >= len(tokens) {
		return ""
	}
	return tokens[pos].Value
}
