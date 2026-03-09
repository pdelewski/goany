package rustparser

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

func charToString(ch int) string {
	if ch == 9 {
		return "\t"
	} else if ch == 10 {
		return "\n"
	} else if ch == 13 {
		return "\r"
	} else if ch == 32 {
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

// Tokenize converts Rust source code into a list of tokens
func Tokenize(input string) []Token {
	var tokens []Token
	pos := 0
	line := 1
	col := 1

	for pos < len(input) {
		ch := int(input[pos])

		// Skip whitespace
		if isWhitespace(ch) {
			pos = pos + 1
			col = col + 1
			continue
		}

		// Newline
		if ch == int('\n') {
			pos = pos + 1
			line = line + 1
			col = 1
			continue
		}

		// Line comment
		if ch == int('/') && pos+1 < len(input) && int(input[pos+1]) == int('/') {
			pos = pos + 2
			for pos < len(input) && int(input[pos]) != int('\n') {
				pos = pos + 1
			}
			continue
		}

		// Identifier or keyword
		if isLetter(ch) {
			startCol := col
			word := ""
			for pos < len(input) && isAlphaNumeric(int(input[pos])) {
				word += charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}
			if isRustKeyword(word) {
				tokens = append(tokens, NewToken(TokenKeyword, word, line, startCol))
			} else {
				tokens = append(tokens, NewToken(TokenIdent, word, line, startCol))
			}
			continue
		}

		// Number
		if isDigit(ch) {
			startCol := col
			num := ""
			for pos < len(input) && isDigit(int(input[pos])) {
				num += charToString(int(input[pos]))
				pos = pos + 1
				col = col + 1
			}
			tokens = append(tokens, NewToken(TokenNumber, num, line, startCol))
			continue
		}

		// String literal
		if ch == int('"') {
			startCol := col
			pos = pos + 1
			col = col + 1
			strVal := ""
			for pos < len(input) && int(input[pos]) != int('"') {
				if int(input[pos]) == int('\\') && pos+1 < len(input) {
					pos = pos + 1
					col = col + 1
					escaped := int(input[pos])
					if escaped == int('n') {
						strVal += "\n"
					} else if escaped == int('t') {
						strVal += "\t"
					} else if escaped == int('\\') {
						strVal += "\\"
					} else {
						strVal += charToString(escaped)
					}
				} else {
					strVal += charToString(int(input[pos]))
				}
				pos = pos + 1
				col = col + 1
			}
			if pos < len(input) {
				pos = pos + 1
				col = col + 1
			}
			tokens = append(tokens, NewToken(TokenString, strVal, line, startCol))
			continue
		}

		// Two-character operators
		if pos+1 < len(input) {
			ch2 := int(input[pos+1])
			if ch == int('-') && ch2 == int('>') {
				tokens = append(tokens, NewToken(TokenArrow, "->", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int(':') && ch2 == int(':') {
				tokens = append(tokens, NewToken(TokenDoubleColon, "::", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('=') && ch2 == int('=') {
				tokens = append(tokens, NewToken(TokenEq, "==", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('!') && ch2 == int('=') {
				tokens = append(tokens, NewToken(TokenNeq, "!=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('<') && ch2 == int('=') {
				tokens = append(tokens, NewToken(TokenLe, "<=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('>') && ch2 == int('=') {
				tokens = append(tokens, NewToken(TokenGe, ">=", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('&') && ch2 == int('&') {
				tokens = append(tokens, NewToken(TokenAndAnd, "&&", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
			if ch == int('|') && ch2 == int('|') {
				tokens = append(tokens, NewToken(TokenOrOr, "||", line, col))
				pos = pos + 2
				col = col + 2
				continue
			}
		}

		// Single-character tokens
		if ch == int('(') {
			tokens = append(tokens, NewToken(TokenLParen, "(", line, col))
		} else if ch == int(')') {
			tokens = append(tokens, NewToken(TokenRParen, ")", line, col))
		} else if ch == int('{') {
			tokens = append(tokens, NewToken(TokenLBrace, "{", line, col))
		} else if ch == int('}') {
			tokens = append(tokens, NewToken(TokenRBrace, "}", line, col))
		} else if ch == int('[') {
			tokens = append(tokens, NewToken(TokenLBracket, "[", line, col))
		} else if ch == int(']') {
			tokens = append(tokens, NewToken(TokenRBracket, "]", line, col))
		} else if ch == int(';') {
			tokens = append(tokens, NewToken(TokenSemicolon, ";", line, col))
		} else if ch == int(',') {
			tokens = append(tokens, NewToken(TokenComma, ",", line, col))
		} else if ch == int(':') {
			tokens = append(tokens, NewToken(TokenColon, ":", line, col))
		} else if ch == int('.') {
			tokens = append(tokens, NewToken(TokenDot, ".", line, col))
		} else if ch == int('=') {
			tokens = append(tokens, NewToken(TokenAssign, "=", line, col))
		} else if ch == int('<') {
			tokens = append(tokens, NewToken(TokenLt, "<", line, col))
		} else if ch == int('>') {
			tokens = append(tokens, NewToken(TokenGt, ">", line, col))
		} else if ch == int('!') {
			tokens = append(tokens, NewToken(TokenExcl, "!", line, col))
		} else if ch == int('+') {
			tokens = append(tokens, NewToken(TokenPlus, "+", line, col))
		} else if ch == int('-') {
			tokens = append(tokens, NewToken(TokenMinus, "-", line, col))
		} else if ch == int('*') {
			tokens = append(tokens, NewToken(TokenStar, "*", line, col))
		} else if ch == int('/') {
			tokens = append(tokens, NewToken(TokenSlash, "/", line, col))
		} else if ch == int('%') {
			tokens = append(tokens, NewToken(TokenPercent, "%", line, col))
		} else if ch == int('&') {
			tokens = append(tokens, NewToken(TokenAmpersand, "&", line, col))
		} else if ch == int('#') {
			tokens = append(tokens, NewToken(TokenHash, "#", line, col))
		}
		pos = pos + 1
		col = col + 1
	}

	tokens = append(tokens, NewToken(TokenEOF, "", line, col))
	return tokens
}
