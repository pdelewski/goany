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

		// Identifiers and keywords
		if isLetter(ch) {
			identStartCol := col
			value := ""
			for pos < len(input) && isAlphaNumeric(int(input[pos])) {
				value = value + charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}
			if isKeyword(value) {
				tokens = append(tokens, NewToken(TokenKeyword, value, line, identStartCol))
			} else {
				tokens = append(tokens, NewToken(TokenIdentifier, value, line, identStartCol))
			}
			continue
		}

		// Numbers
		if isDigit(ch) {
			numStartCol := col
			value := ""
			for pos < len(input) && (isDigit(int(input[pos])) || int(input[pos]) == int('.')) {
				value = value + charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}
			tokens = append(tokens, NewToken(TokenNumber, value, line, numStartCol))
			continue
		}

		// String literals
		if ch == int('"') || ch == int('\'') {
			quote := ch
			strStartCol := col
			value := ""
			pos = pos + 1 // Skip opening quote
			col = col + 1
			for pos < len(input) && int(input[pos]) != quote {
				if int(input[pos]) == int('\\') && pos+1 < len(input) {
					// Handle escape sequences
					pos = pos + 1
					col = col + 1
					escaped := int(input[pos])
					if escaped == int('n') {
						value = value + "\n"
					} else if escaped == int('t') {
						value = value + "\t"
					} else if escaped == int('\\') {
						value = value + "\\"
					} else if escaped == int('"') {
						value = value + "\""
					} else if escaped == int('\'') {
						value = value + "'"
					} else {
						value = value + charToString(escaped)
					}
				} else {
					value = value + charToString(int(input[pos]))
				}
				pos = pos + 1
				col = col + 1
			}
			if pos < len(input) {
				pos = pos + 1 // Skip closing quote
				col = col + 1
			}
			tokens = append(tokens, NewToken(TokenString, value, line, strStartCol))
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
