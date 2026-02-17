package pyparser

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
	return ch == int(' ') || ch == int('\t')
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

func toLower(s string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		ch := int(s[i])
		if ch >= int('A') && ch <= int('Z') {
			ch = ch + 32
		}
		result = result + charToString(ch)
	}
	return result
}

// tokenizeString handles all string types including triple-quoted, raw, byte, f-strings
func tokenizeString(input string, pos int, col int, line int, prefix string) (Token, int, int, int) {
	strStartCol := col
	strStartLine := line
	quote := int(input[pos])
	isTriple := false
	isRaw := false
	isFString := false
	isByte := false

	// Check prefix flags
	for i := 0; i < len(prefix); i++ {
		ch := int(prefix[i])
		if ch == int('r') {
			isRaw = true
		} else if ch == int('f') {
			isFString = true
		} else if ch == int('b') {
			isByte = true
		}
	}

	// Check for triple quotes
	if pos+2 < len(input) && int(input[pos+1]) == quote && int(input[pos+2]) == quote {
		isTriple = true
		pos = pos + 3
		col = col + 3
	} else {
		pos = pos + 1
		col = col + 1
	}

	value := ""
	if isFString {
		value = "f:"
	} else if isByte {
		value = "b:"
	} else if isRaw {
		value = "r:"
	}

	// Parse string content
	for pos < len(input) {
		ch := int(input[pos])

		// Check for end of string
		if isTriple {
			if ch == quote && pos+2 < len(input) && int(input[pos+1]) == quote && int(input[pos+2]) == quote {
				pos = pos + 3
				col = col + 3
				break
			}
		} else {
			if ch == quote {
				pos = pos + 1
				col = col + 1
				break
			}
		}

		// Handle newlines in triple-quoted strings
		if ch == int('\n') {
			if isTriple {
				value = value + "\n"
				pos = pos + 1
				line = line + 1
				col = 1
				continue
			} else {
				// Unterminated string
				break
			}
		}

		// Handle escape sequences (unless raw string)
		if ch == int('\\') && !isRaw && pos+1 < len(input) {
			pos = pos + 1
			col = col + 1
			escaped := int(input[pos])
			if escaped == int('n') {
				value = value + "\n"
			} else if escaped == int('t') {
				value = value + "\t"
			} else if escaped == int('r') {
				value = value + "\r"
			} else if escaped == int('\\') {
				value = value + "\\"
			} else if escaped == int('"') {
				value = value + "\""
			} else if escaped == int('\'') {
				value = value + "'"
			} else if escaped == int('0') {
				value = value + charToString(0)
			} else if escaped == int('\n') {
				// Line continuation
				line = line + 1
				col = 0
			} else {
				value = value + charToString(escaped)
			}
			pos = pos + 1
			col = col + 1
			continue
		}

		// Handle f-string expressions
		if isFString && ch == int('{') {
			if pos+1 < len(input) && int(input[pos+1]) == int('{') {
				// Escaped brace
				value = value + "{"
				pos = pos + 2
				col = col + 2
				continue
			}
			// Parse expression inside braces
			value = value + "|EXPR:"
			pos = pos + 1
			col = col + 1
			braceDepth := 1
			for pos < len(input) && braceDepth > 0 {
				exprCh := int(input[pos])
				if exprCh == int('{') {
					braceDepth = braceDepth + 1
				} else if exprCh == int('}') {
					braceDepth = braceDepth - 1
					if braceDepth == 0 {
						pos = pos + 1
						col = col + 1
						break
					}
				}
				value = value + charToString(exprCh)
				pos = pos + 1
				col = col + 1
			}
			value = value + "|STR:"
			continue
		}

		if isFString && ch == int('}') {
			if pos+1 < len(input) && int(input[pos+1]) == int('}') {
				// Escaped brace
				value = value + "}"
				pos = pos + 2
				col = col + 2
				continue
			}
		}

		value = value + charToString(ch)
		pos = pos + 1
		col = col + 1
	}

	return NewToken(TokenString, value, strStartLine, strStartCol), pos, col, line
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

// Tokenize converts Python source code into a slice of tokens
func Tokenize(input string) []Token {
	var tokens []Token
	var indentStack []int
	indentStack = append(indentStack, 0) // Start with indent level 0

	pos := 0
	line := 1
	col := 1
	atLineStart := true

	for pos < len(input) {
		ch := int(input[pos])

		// Handle newlines
		if ch == int('\n') {
			tokens = append(tokens, NewToken(TokenNewline, "\\n", line, col))
			pos = pos + 1
			line = line + 1
			col = 1
			atLineStart = true
			continue
		}

		// Handle carriage return (skip it)
		if ch == int('\r') {
			pos = pos + 1
			continue
		}

		// Handle indentation at line start
		if atLineStart {
			indent := 0
			for pos < len(input) {
				ch = int(input[pos])
				if ch == int(' ') {
					indent = indent + 1
					pos = pos + 1
					col = col + 1
				} else if ch == int('\t') {
					indent = indent + 4 // Treat tab as 4 spaces
					pos = pos + 1
					col = col + 1
				} else {
					break
				}
			}

			// Skip blank lines and comment-only lines
			if pos < len(input) {
				ch = int(input[pos])
				if ch == int('\n') || ch == int('#') {
					if ch == int('#') {
						// Skip comment
						for pos < len(input) && int(input[pos]) != int('\n') {
							pos = pos + 1
						}
					}
					continue
				}
			}

			// Process indentation changes
			// Need to check even when indent is 0 to handle dedent from indented blocks
			currentIndent := indentStack[len(indentStack)-1]
			if indent > currentIndent {
				indentStack = append(indentStack, indent)
				tokens = append(tokens, NewToken(TokenIndent, "", line, 1))
			} else if indent < currentIndent {
				// Emit DEDENT tokens for each level we're leaving
				for len(indentStack) > 1 && indentStack[len(indentStack)-1] > indent {
					indentStack = indentStack[:len(indentStack)-1]
					tokens = append(tokens, NewToken(TokenDedent, "", line, 1))
				}
			}

			atLineStart = false
			if pos >= len(input) {
				break
			}
			ch = int(input[pos])
		}

		// Skip whitespace (not at line start)
		if isWhitespace(ch) {
			pos = pos + 1
			col = col + 1
			continue
		}

		// Skip comments
		if ch == int('#') {
			for pos < len(input) && int(input[pos]) != int('\n') {
				pos = pos + 1
			}
			continue
		}

		// Identifiers and keywords (also handles string prefixes like r, b, f, rf, rb, br, fr)
		if isLetter(ch) {
			identStartCol := col
			value := ""
			for pos < len(input) && isAlphaNumeric(int(input[pos])) {
				value = value + charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}

			// Check for string prefix (r, b, f, rf, rb, br, fr, R, B, F, etc.)
			if pos < len(input) && (int(input[pos]) == int('"') || int(input[pos]) == int('\'')) {
				lowerValue := toLower(value)
				if lowerValue == "r" || lowerValue == "b" || lowerValue == "f" ||
					lowerValue == "rf" || lowerValue == "fr" || lowerValue == "rb" ||
					lowerValue == "br" {
					// This is a prefixed string
					prefix := lowerValue
					var strToken Token
					strToken, pos, col, line = tokenizeString(input, pos, col, line, prefix)
					tokens = append(tokens, strToken)
					continue
				}
			}

			if isKeyword(value) {
				tokens = append(tokens, NewToken(TokenKeyword, value, line, identStartCol))
			} else {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
			}
			continue
		}

		// Numbers (including hex 0x, octal 0o, binary 0b, underscores, complex j)
		if isDigit(ch) {
			numStartCol := col
			value := ""

			// Check for hex, octal, binary prefixes
			if ch == int('0') && pos+1 < len(input) {
				nextCh := int(input[pos+1])
				if nextCh == int('x') || nextCh == int('X') {
					// Hexadecimal
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
					continue
				} else if nextCh == int('o') || nextCh == int('O') {
					// Octal
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
					continue
				} else if nextCh == int('b') || nextCh == int('B') {
					// Binary
					value = "0b"
					pos = pos + 2
					col = col + 2
					for pos < len(input) && (int(input[pos]) == int('0') || int(input[pos]) == int('1') || int(input[pos]) == int('_')) {
						if int(input[pos]) != int('_') {
							value = value + charToString(int(input[pos]))
						}
						pos = pos + 1
						col = col + 1
					}
					tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
					continue
				}
			}

			// Regular decimal number (with underscores, decimal point, exponent, complex)
			for pos < len(input) {
				currCh := int(input[pos])
				if isDigit(currCh) || currCh == int('.') || currCh == int('_') {
					if currCh != int('_') {
						value = value + charToString(currCh)
					}
					pos = pos + 1
					col = col + 1
				} else if currCh == int('e') || currCh == int('E') {
					// Exponent
					value = value + charToString(currCh)
					pos = pos + 1
					col = col + 1
					// Optional sign after exponent
					if pos < len(input) && (int(input[pos]) == int('+') || int(input[pos]) == int('-')) {
						value = value + charToString(int(input[pos]))
						pos = pos + 1
						col = col + 1
					}
				} else if currCh == int('j') || currCh == int('J') {
					// Complex number
					value = value + "j"
					pos = pos + 1
					col = col + 1
					break
				} else {
					break
				}
			}
			tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
			continue
		}

		// String literals (including triple-quoted)
		if ch == int('"') || ch == int('\'') {
			var strToken Token
			strToken, pos, col, line = tokenizeString(input, pos, col, line, "")
			tokens = append(tokens, strToken)
			continue
		}

		// Three-character operators (ellipsis)
		if pos+2 < len(input) && ch == int('.') && int(input[pos+1]) == int('.') && int(input[pos+2]) == int('.') {
			tokens = append(tokens, NewToken(TokenEllipsis, "...", line, col))
			pos = pos + 3
			col = col + 3
			continue
		}

		// Two-character operators
		if pos+1 < len(input) {
			twoChar := charToString(ch) + charToString(int(input[pos+1]))
			if twoChar == "==" || twoChar == "!=" || twoChar == "<=" || twoChar == ">=" || twoChar == "//" {
				tokens = append(tokens, NewToken(TokenOperator, twoChar, line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "**" {
				tokens = append(tokens, NewToken(TokenDoubleStar, "**", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "->" {
				tokens = append(tokens, NewToken(TokenArrow, "->", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "+=" {
				tokens = append(tokens, NewToken(TokenPlusAssign, "+=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "-=" {
				tokens = append(tokens, NewToken(TokenMinusAssign, "-=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "*=" {
				tokens = append(tokens, NewToken(TokenStarAssign, "*=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "/=" {
				tokens = append(tokens, NewToken(TokenSlashAssign, "/=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "%=" {
				tokens = append(tokens, NewToken(TokenPercentAssign, "%=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == "<<" {
				tokens = append(tokens, NewToken(TokenLeftShift, "<<", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == ">>" {
				tokens = append(tokens, NewToken(TokenRightShift, ">>", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if twoChar == ":=" {
				tokens = append(tokens, NewToken(TokenWalrus, ":=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
		}

		// Single-character tokens
		charStartCol := col
		if ch == int(':') {
			tokens = append(tokens, NewToken(TokenColon, ":", line, charStartCol))
		} else if ch == int(',') {
			tokens = append(tokens, NewToken(TokenComma, ",", line, charStartCol))
		} else if ch == int('(') {
			tokens = append(tokens, NewToken(TokenLParen, "(", line, charStartCol))
		} else if ch == int(')') {
			tokens = append(tokens, NewToken(TokenRParen, ")", line, charStartCol))
		} else if ch == int('[') {
			tokens = append(tokens, NewToken(TokenLBracket, "[", line, charStartCol))
		} else if ch == int(']') {
			tokens = append(tokens, NewToken(TokenRBracket, "]", line, charStartCol))
		} else if ch == int('{') {
			tokens = append(tokens, NewToken(TokenLBrace, "{", line, charStartCol))
		} else if ch == int('}') {
			tokens = append(tokens, NewToken(TokenRBrace, "}", line, charStartCol))
		} else if ch == int('.') {
			tokens = append(tokens, NewToken(TokenDot, ".", line, charStartCol))
		} else if ch == int('=') {
			tokens = append(tokens, NewToken(TokenAssign, "=", line, charStartCol))
		} else if ch == int('@') {
			tokens = append(tokens, NewToken(TokenAt, "@", line, charStartCol))
		} else if ch == int('*') {
			tokens = append(tokens, NewToken(TokenStar, "*", line, charStartCol))
		} else if ch == int('+') || ch == int('-') || ch == int('/') || ch == int('%') || ch == int('<') || ch == int('>') {
			tokens = append(tokens, NewToken(TokenOperator, charToString(ch), line, charStartCol))
		} else if ch == int('&') {
			tokens = append(tokens, NewToken(TokenAmpersand, "&", line, charStartCol))
		} else if ch == int('|') {
			tokens = append(tokens, NewToken(TokenPipe, "|", line, charStartCol))
		} else if ch == int('^') {
			tokens = append(tokens, NewToken(TokenCaret, "^", line, charStartCol))
		} else if ch == int('~') {
			tokens = append(tokens, NewToken(TokenTilde, "~", line, charStartCol))
		} else {
			// Unknown character - skip it
		}
		pos = pos + 1
		col = col + 1
	}

	// Emit remaining DEDENTs at end of file
	for len(indentStack) > 1 {
		indentStack = indentStack[:len(indentStack)-1]
		tokens = append(tokens, NewToken(TokenDedent, "", line, col))
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
