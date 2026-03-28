package arm64asm

// Token types
const (
	TokEOF     = 0
	TokIdent   = 1
	TokNumber  = 2
	TokHash    = 3
	TokComma   = 4
	TokLBrack  = 5
	TokRBrack  = 6
	TokBang    = 7
	TokColon   = 8
	TokNewline = 9
	TokMinus   = 10
	TokString  = 11
	TokDot     = 12
)

// Operand kinds
const (
	OpndNone    = 0
	OpndReg     = 1
	OpndImm     = 2
	OpndLabel   = 3
	OpndMem     = 4 // [Xn, #imm]
	OpndMemPre  = 5 // [Xn, #imm]!
	OpndMemPost = 6 // [Xn], #imm
	OpndShift   = 7
	OpndRegPair = 8 // for LDP/STP
)

// Shift types
const (
	ShiftLSL = 0
	ShiftLSR = 1
	ShiftASR = 2
)

// Condition codes
const (
	CondEQ = 0  // equal
	CondNE = 1  // not equal
	CondCS = 2  // carry set / HS (unsigned >=)
	CondCC = 3  // carry clear / LO (unsigned <)
	CondMI = 4  // minus / negative
	CondPL = 5  // plus / positive or zero
	CondVS = 6  // overflow
	CondVC = 7  // no overflow
	CondHI = 8  // unsigned higher
	CondLS = 9  // unsigned lower or same
	CondGE = 10 // signed >=
	CondLT = 11 // signed <
	CondGT = 12 // signed >
	CondLE = 13 // signed <=
	CondAL = 14 // always
)

// Instruction kinds
const (
	InstrKindNone      = 0
	InstrKindInstr     = 1
	InstrKindDirective = 2
	InstrKindLabel     = 3
)

// Token represents a lexical token from assembly source
type Token struct {
	Type int
	Text string
}

// Operand represents a parsed instruction operand
type Operand struct {
	Kind      int
	Reg       int
	Reg2      int
	Imm       int
	Label     string
	Is32      bool
	ShiftType int
	ShiftAmt  int
}

// ParsedInstr represents a fully parsed assembly line
type ParsedInstr struct {
	Kind     int
	Mnemonic string
	Operands []Operand
	Label    string
	CondCode int
	RawBytes []byte
	ByteSize int
	Offset   int
}

// LabelEntry stores a label name and its byte offset
type LabelEntry struct {
	Name   string
	Offset int
}

// --- Character classification ---

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAlnum(b byte) bool {
	return isAlpha(b) || isDigit(b)
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r'
}

// toUpper converts a single byte to uppercase
func toUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

// strUpper returns an uppercase copy of s
func strUpper(s string) string {
	result := ""
	i := 0
	for {
		if i >= len(s) {
			break
		}
		ch := byte(s[i])
		result = result + string(toUpper(ch))
		i = i + 1
	}
	return result
}

// strEqual compares two strings for equality
func strEqual(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	i := 0
	for {
		if i >= len(a) {
			break
		}
		if byte(a[i]) != byte(b[i]) {
			return false
		}
		i = i + 1
	}
	return true
}

// strEqualUpper compares two strings case-insensitively (b must be uppercase)
func strEqualUpper(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	i := 0
	for {
		if i >= len(a) {
			break
		}
		ca := byte(a[i])
		cb := byte(b[i])
		if toUpper(ca) != cb {
			return false
		}
		i = i + 1
	}
	return true
}

// parseDecimal parses a decimal string to int
func parseDecimal(s string) int {
	result := 0
	i := 0
	for {
		if i >= len(s) {
			break
		}
		result = result*10 + int(byte(s[i])-'0')
		i = i + 1
	}
	return result
}

// parseHex parses a hex string (without 0x prefix) to int
func parseHex(s string) int {
	result := 0
	i := 0
	for {
		if i >= len(s) {
			break
		}
		b := byte(s[i])
		result = result * 16
		if b >= '0' && b <= '9' {
			result = result + int(b-'0')
		} else if b >= 'a' && b <= 'f' {
			result = result + int(b-'a'+10)
		} else if b >= 'A' && b <= 'F' {
			result = result + int(b-'A'+10)
		}
		i = i + 1
	}
	return result
}

// parseNumber parses a number token text (decimal or 0x hex)
func parseNumber(s string) int {
	if len(s) > 2 && byte(s[0]) == '0' && (byte(s[1]) == 'x' || byte(s[1]) == 'X') {
		// Build hex substring without 0x prefix
		hex := ""
		i := 2
		for {
			if i >= len(s) {
				break
			}
			hex = hex + string(s[i])
			i = i + 1
		}
		return parseHex(hex)
	}
	return parseDecimal(s)
}

// --- Tokenizer ---

// Tokenize converts assembly text into a flat list of tokens
func Tokenize(text string) []Token {
	tokens := []Token{}
	i := 0
	for {
		if i >= len(text) {
			break
		}
		b := byte(text[i])

		// Skip whitespace (not newline)
		if isWhitespace(b) {
			i = i + 1
			continue
		}

		// Newline
		if b == '\n' {
			tokens = append(tokens, Token{Type: TokNewline, Text: "\n"})
			i = i + 1
			continue
		}

		// Comment: // or ;
		if b == ';' {
			// skip to end of line
			for {
				if i >= len(text) {
					break
				}
				if byte(text[i]) == '\n' {
					break
				}
				i = i + 1
			}
			continue
		}
		if b == '/' && i+1 < len(text) && byte(text[i+1]) == '/' {
			// skip to end of line
			for {
				if i >= len(text) {
					break
				}
				if byte(text[i]) == '\n' {
					break
				}
				i = i + 1
			}
			continue
		}

		// Hash
		if b == '#' {
			tokens = append(tokens, Token{Type: TokHash, Text: "#"})
			i = i + 1
			continue
		}

		// Comma
		if b == ',' {
			tokens = append(tokens, Token{Type: TokComma, Text: ","})
			i = i + 1
			continue
		}

		// Left bracket
		if b == '[' {
			tokens = append(tokens, Token{Type: TokLBrack, Text: "["})
			i = i + 1
			continue
		}

		// Right bracket
		if b == ']' {
			tokens = append(tokens, Token{Type: TokRBrack, Text: "]"})
			i = i + 1
			continue
		}

		// Bang (pre-index marker)
		if b == '!' {
			tokens = append(tokens, Token{Type: TokBang, Text: "!"})
			i = i + 1
			continue
		}

		// Colon
		if b == ':' {
			tokens = append(tokens, Token{Type: TokColon, Text: ":"})
			i = i + 1
			continue
		}

		// Minus sign
		if b == '-' {
			tokens = append(tokens, Token{Type: TokMinus, Text: "-"})
			i = i + 1
			continue
		}

		// String literal (for .ascii/.asciz directives)
		if b == '"' {
			i = i + 1
			str := ""
			for {
				if i >= len(text) {
					break
				}
				if byte(text[i]) == '"' {
					i = i + 1
					break
				}
				if byte(text[i]) == '\\' && i+1 < len(text) {
					nc := byte(text[i+1])
					if nc == 'n' {
						str = str + "\n" // newline
						i = i + 2
						continue
					} else if nc == 't' {
						str = str + "\t" // tab
						i = i + 2
						continue
					} else if nc == '\\' {
						str = str + "\\"
						i = i + 2
						continue
					} else if nc == '"' {
						str = str + "\""
						i = i + 2
						continue
					} else if nc == '0' {
						// null byte - not representable in string for transpilation
						i = i + 2
						continue
					}
				}
				str = str + string(text[i])
				i = i + 1
			}
			tokens = append(tokens, Token{Type: TokString, Text: str})
			continue
		}

		// Number: starts with digit
		if isDigit(b) {
			numText := ""
			// Check for 0x hex prefix
			if b == '0' && i+1 < len(text) && (byte(text[i+1]) == 'x' || byte(text[i+1]) == 'X') {
				numText = numText + string(text[i]) + string(text[i+1])
				i = i + 2
				for {
					if i >= len(text) {
						break
					}
					c := byte(text[i])
					isHex := isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
					if !isHex {
						break
					}
					numText = numText + string(c)
					i = i + 1
				}
			} else {
				for {
					if i >= len(text) {
						break
					}
					if !isDigit(byte(text[i])) {
						break
					}
					numText = numText + string(text[i])
					i = i + 1
				}
			}
			tokens = append(tokens, Token{Type: TokNumber, Text: numText})
			continue
		}

		// Dot (directive start or conditional branch separator)
		if b == '.' {
			// Check if this is a directive: .global, .byte, etc.
			if i+1 < len(text) && isAlpha(byte(text[i+1])) {
				i = i + 1
				identText := "."
				for {
					if i >= len(text) {
						break
					}
					if !isAlnum(byte(text[i])) {
						break
					}
					identText = identText + string(text[i])
					i = i + 1
				}
				tokens = append(tokens, Token{Type: TokIdent, Text: identText})
				continue
			}
			tokens = append(tokens, Token{Type: TokDot, Text: "."})
			i = i + 1
			continue
		}

		// Identifier: alpha or dot-prefixed
		if isAlpha(b) {
			identText := ""
			for {
				if i >= len(text) {
					break
				}
				c := byte(text[i])
				if !isAlnum(c) && c != '.' {
					break
				}
				identText = identText + string(c)
				i = i + 1
			}
			tokens = append(tokens, Token{Type: TokIdent, Text: identText})
			continue
		}

		// Unknown, skip
		i = i + 1
	}

	return tokens
}

// --- Register parsing ---

// parseRegister attempts to parse a register name, returns (regNum, is32bit, ok)
func parseRegister(name string) (int, bool, bool) {
	upper := strUpper(name)

	// Stack pointer
	if strEqual(upper, "SP") {
		return 31, false, true
	}

	// Zero registers
	if strEqual(upper, "XZR") {
		return 31, false, true
	}
	if strEqual(upper, "WZR") {
		return 31, true, true
	}

	// X registers (64-bit)
	if len(upper) >= 2 && byte(upper[0]) == 'X' {
		numPart := ""
		k := 1
		for {
			if k >= len(upper) {
				break
			}
			numPart = numPart + string(upper[k])
			k = k + 1
		}
		if len(numPart) > 0 && isDigit(byte(numPart[0])) {
			n := parseDecimal(numPart)
			if n >= 0 && n <= 30 {
				return n, false, true
			}
		}
	}

	// W registers (32-bit)
	if len(upper) >= 2 && byte(upper[0]) == 'W' {
		numPart := ""
		k := 1
		for {
			if k >= len(upper) {
				break
			}
			numPart = numPart + string(upper[k])
			k = k + 1
		}
		if len(numPart) > 0 && isDigit(byte(numPart[0])) {
			n := parseDecimal(numPart)
			if n >= 0 && n <= 30 {
				return n, true, true
			}
		}
	}

	return 0, false, false
}

// parseCondCode maps a condition suffix to a condition code
func parseCondCode(suffix string) int {
	upper := strUpper(suffix)
	if strEqual(upper, "EQ") {
		return CondEQ
	}
	if strEqual(upper, "NE") {
		return CondNE
	}
	if strEqual(upper, "CS") || strEqual(upper, "HS") {
		return CondCS
	}
	if strEqual(upper, "CC") || strEqual(upper, "LO") {
		return CondCC
	}
	if strEqual(upper, "MI") {
		return CondMI
	}
	if strEqual(upper, "PL") {
		return CondPL
	}
	if strEqual(upper, "VS") {
		return CondVS
	}
	if strEqual(upper, "VC") {
		return CondVC
	}
	if strEqual(upper, "HI") {
		return CondHI
	}
	if strEqual(upper, "LS") {
		return CondLS
	}
	if strEqual(upper, "GE") {
		return CondGE
	}
	if strEqual(upper, "LT") {
		return CondLT
	}
	if strEqual(upper, "GT") {
		return CondGT
	}
	if strEqual(upper, "LE") {
		return CondLE
	}
	if strEqual(upper, "AL") {
		return CondAL
	}
	return -1
}

// parseShiftType maps a shift name to a shift type constant
func parseShiftType(name string) int {
	upper := strUpper(name)
	if strEqual(upper, "LSL") {
		return ShiftLSL
	}
	if strEqual(upper, "LSR") {
		return ShiftLSR
	}
	if strEqual(upper, "ASR") {
		return ShiftASR
	}
	return -1
}

// --- Parser ---

// parseLine parses a single line of tokens starting at position pos, returns (ParsedInstr, newPos)
func parseLine(tokens []Token, pos int) (ParsedInstr, int) {
	instr := ParsedInstr{Kind: InstrKindNone, CondCode: -1}

	// Skip leading newlines
	for {
		if pos >= len(tokens) {
			break
		}
		if tokens[pos].Type != TokNewline {
			break
		}
		pos = pos + 1
	}
	if pos >= len(tokens) {
		return instr, pos
	}

	// Check for label: identifier followed by colon
	if tokens[pos].Type == TokIdent && pos+1 < len(tokens) && tokens[pos+1].Type == TokColon {
		instr.Label = tokens[pos].Text
		pos = pos + 2
		// Skip whitespace/newlines after label
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type != TokNewline {
				break
			}
			pos = pos + 1
		}
		if pos >= len(tokens) {
			instr.Kind = InstrKindLabel
			instr.ByteSize = 0
			return instr, pos
		}
		// Check if just a label on its own line (next is newline or eof)
		if tokens[pos].Type == TokNewline {
			instr.Kind = InstrKindLabel
			instr.ByteSize = 0
			return instr, pos
		}
	}

	// If no more tokens, return label-only
	if pos >= len(tokens) {
		if instr.Label != "" {
			instr.Kind = InstrKindLabel
			instr.ByteSize = 0
		}
		return instr, pos
	}

	// Expect mnemonic or directive
	if tokens[pos].Type != TokIdent {
		// skip to next newline
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type == TokNewline {
				break
			}
			pos = pos + 1
		}
		return instr, pos
	}

	mnemonic := strUpper(tokens[pos].Text)
	pos = pos + 1

	// Check for directive
	if len(mnemonic) > 0 && byte(mnemonic[0]) == '.' {
		instr.Kind = InstrKindDirective
		instr.Mnemonic = mnemonic
		instr, pos = parseDirective(instr, tokens, pos)
		return instr, pos
	}

	// Check for conditional branch: B.cond
	condCode := -1
	// B.EQ, B.NE etc come through as a single ident "B.EQ"
	if len(mnemonic) > 2 && byte(mnemonic[0]) == 'B' && byte(mnemonic[1]) == '.' {
		suffix := ""
		k := 2
		for {
			if k >= len(mnemonic) {
				break
			}
			suffix = suffix + string(mnemonic[k])
			k = k + 1
		}
		condCode = parseCondCode(suffix)
		if condCode >= 0 {
			mnemonic = "B.COND"
		}
	}

	instr.Kind = InstrKindInstr
	instr.Mnemonic = mnemonic
	instr.CondCode = condCode
	instr.ByteSize = 4 // all ARM64 instructions are 4 bytes

	// Parse operands
	operands := []Operand{}
	for {
		if pos >= len(tokens) {
			break
		}
		if tokens[pos].Type == TokNewline {
			break
		}

		op, newPos := parseOperand(tokens, pos)
		if op.Kind == OpndNone {
			// skip unknown token
			pos = newPos
			continue
		}
		operands = append(operands, op)
		pos = newPos

		// comma between operands
		if pos < len(tokens) && tokens[pos].Type == TokComma {
			pos = pos + 1
		}
	}
	instr.Operands = operands

	return instr, pos
}

// parseOperand parses a single operand starting at pos
func parseOperand(tokens []Token, pos int) (Operand, int) {
	op := Operand{Kind: OpndNone}

	if pos >= len(tokens) {
		return op, pos
	}

	tok := tokens[pos]

	// Memory operand: [Xn, #imm] or [Xn, #imm]! or [Xn] (for post-index, handled differently)
	if tok.Type == TokLBrack {
		return parseMemOperand(tokens, pos)
	}

	// Immediate: #imm or #-imm
	if tok.Type == TokHash {
		pos = pos + 1
		negative := false
		if pos < len(tokens) && tokens[pos].Type == TokMinus {
			negative = true
			pos = pos + 1
		}
		if pos < len(tokens) && tokens[pos].Type == TokNumber {
			val := parseNumber(tokens[pos].Text)
			if negative {
				val = -val
			}
			pos = pos + 1
			op.Kind = OpndImm
			op.Imm = val
			return op, pos
		}
		// Just # with no number — return as zero imm
		op.Kind = OpndImm
		op.Imm = 0
		return op, pos
	}

	// Register or label or shift
	if tok.Type == TokIdent {
		// Try register
		regNum, is32, ok := parseRegister(tok.Text)
		if ok {
			op.Kind = OpndReg
			op.Reg = regNum
			op.Is32 = is32
			pos = pos + 1

			// Check for optional shift after register: LSL #n
			if pos < len(tokens) && tokens[pos].Type == TokComma {
				// peek ahead
				if pos+1 < len(tokens) && tokens[pos+1].Type == TokIdent {
					st := parseShiftType(tokens[pos+1].Text)
					if st >= 0 {
						pos = pos + 2 // skip comma and shift name
						// expect #imm
						if pos < len(tokens) && tokens[pos].Type == TokHash {
							pos = pos + 1
							if pos < len(tokens) && tokens[pos].Type == TokNumber {
								op.ShiftType = st
								op.ShiftAmt = parseNumber(tokens[pos].Text)
								pos = pos + 1
							}
						}
					}
				}
			}
			return op, pos
		}

		// Try shift mnemonic as operand (for LSL/LSR/ASR used as shift instructions)
		st := parseShiftType(tok.Text)
		if st >= 0 {
			op.Kind = OpndShift
			op.ShiftType = st
			pos = pos + 1
			// Expect #imm
			if pos < len(tokens) && tokens[pos].Type == TokHash {
				pos = pos + 1
				if pos < len(tokens) && tokens[pos].Type == TokNumber {
					op.ShiftAmt = parseNumber(tokens[pos].Text)
					pos = pos + 1
				}
			}
			return op, pos
		}

		// Otherwise it's a label
		op.Kind = OpndLabel
		op.Label = tok.Text
		pos = pos + 1
		return op, pos
	}

	// Number without hash (could be label-like or direct)
	if tok.Type == TokNumber {
		op.Kind = OpndImm
		op.Imm = parseNumber(tok.Text)
		pos = pos + 1
		return op, pos
	}

	// Unknown — skip
	return op, pos + 1
}

// parseMemOperand parses memory operand [Xn], [Xn, #imm], [Xn, #imm]!, or returns base for post-index
func parseMemOperand(tokens []Token, pos int) (Operand, int) {
	op := Operand{Kind: OpndNone}

	// skip '['
	pos = pos + 1

	if pos >= len(tokens) || tokens[pos].Type != TokIdent {
		return op, pos
	}

	// Base register
	baseReg, _, ok := parseRegister(tokens[pos].Text)
	if !ok {
		return op, pos
	}
	pos = pos + 1

	imm := 0

	// Check for comma (offset)
	if pos < len(tokens) && tokens[pos].Type == TokComma {
		pos = pos + 1

		// Parse #imm
		if pos < len(tokens) && tokens[pos].Type == TokHash {
			pos = pos + 1
			negative := false
			if pos < len(tokens) && tokens[pos].Type == TokMinus {
				negative = true
				pos = pos + 1
			}
			if pos < len(tokens) && tokens[pos].Type == TokNumber {
				imm = parseNumber(tokens[pos].Text)
				if negative {
					imm = -imm
				}
				pos = pos + 1
			}
		}
	}

	// Expect ']'
	if pos < len(tokens) && tokens[pos].Type == TokRBrack {
		pos = pos + 1
	}

	// Check for '!' (pre-index)
	if pos < len(tokens) && tokens[pos].Type == TokBang {
		pos = pos + 1
		op.Kind = OpndMemPre
		op.Reg = baseReg // base register
		op.Imm = imm
		return op, pos
	}

	// Check for post-index: ], #imm
	if pos < len(tokens) && tokens[pos].Type == TokComma {
		pos = pos + 1
		if pos < len(tokens) && tokens[pos].Type == TokHash {
			pos = pos + 1
			negative := false
			if pos < len(tokens) && tokens[pos].Type == TokMinus {
				negative = true
				pos = pos + 1
			}
			if pos < len(tokens) && tokens[pos].Type == TokNumber {
				postImm := parseNumber(tokens[pos].Text)
				if negative {
					postImm = -postImm
				}
				pos = pos + 1
				op.Kind = OpndMemPost
				op.Reg = baseReg
				op.Imm = postImm
				return op, pos
			}
		}
	}

	// Plain offset: [Xn, #imm] or [Xn]
	op.Kind = OpndMem
	op.Reg = baseReg
	op.Imm = imm
	return op, pos
}

// parseDirective handles directive parsing
func parseDirective(instr ParsedInstr, tokens []Token, pos int) (ParsedInstr, int) {
	if strEqual(instr.Mnemonic, ".GLOBAL") || strEqual(instr.Mnemonic, ".GLOBL") {
		if pos < len(tokens) && tokens[pos].Type == TokIdent {
			instr.Operands = append(instr.Operands, Operand{Kind: OpndLabel, Label: tokens[pos].Text})
			pos = pos + 1
		}
		instr.ByteSize = 0
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".ALIGN") {
		if pos < len(tokens) && tokens[pos].Type == TokNumber {
			instr.Operands = append(instr.Operands, Operand{Kind: OpndImm, Imm: parseNumber(tokens[pos].Text)})
			pos = pos + 1
		}
		// Alignment is calculated during label resolution
		instr.ByteSize = 0
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".BYTE") {
		bytes := []byte{}
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type == TokNewline {
				break
			}
			if tokens[pos].Type == TokNumber {
				bytes = append(bytes, byte(parseNumber(tokens[pos].Text)))
				pos = pos + 1
			} else if tokens[pos].Type == TokComma {
				pos = pos + 1
			} else {
				pos = pos + 1
			}
		}
		instr.RawBytes = bytes
		instr.ByteSize = len(bytes)
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".HWORD") {
		bytes := []byte{}
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type == TokNewline {
				break
			}
			if tokens[pos].Type == TokNumber {
				val := parseNumber(tokens[pos].Text)
				bytes = append(bytes, byte(val&0xFF))
				bytes = append(bytes, byte((val>>8)&0xFF))
				pos = pos + 1
			} else if tokens[pos].Type == TokComma {
				pos = pos + 1
			} else {
				pos = pos + 1
			}
		}
		instr.RawBytes = bytes
		instr.ByteSize = len(bytes)
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".WORD") {
		bytes := []byte{}
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type == TokNewline {
				break
			}
			if tokens[pos].Type == TokNumber {
				val := parseNumber(tokens[pos].Text)
				bytes = append(bytes, byte(val&0xFF))
				bytes = append(bytes, byte((val>>8)&0xFF))
				bytes = append(bytes, byte((val>>16)&0xFF))
				bytes = append(bytes, byte((val>>24)&0xFF))
				pos = pos + 1
			} else if tokens[pos].Type == TokComma {
				pos = pos + 1
			} else {
				pos = pos + 1
			}
		}
		instr.RawBytes = bytes
		instr.ByteSize = len(bytes)
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".QUAD") {
		bytes := []byte{}
		for {
			if pos >= len(tokens) {
				break
			}
			if tokens[pos].Type == TokNewline {
				break
			}
			if tokens[pos].Type == TokNumber {
				val := parseNumber(tokens[pos].Text)
				// Split into low and high 32-bit halves to avoid C++ shift overflow
				lo := val & 0x7FFFFFFF
				hi := (val >> 31) >> 1 // safe two-step shift for C++ 32-bit int
				bytes = append(bytes, byte(lo&0xFF))
				bytes = append(bytes, byte((lo>>8)&0xFF))
				bytes = append(bytes, byte((lo>>16)&0xFF))
				bytes = append(bytes, byte((lo>>24)&0xFF))
				bytes = append(bytes, byte(hi&0xFF))
				bytes = append(bytes, byte((hi>>8)&0xFF))
				bytes = append(bytes, byte((hi>>16)&0xFF))
				bytes = append(bytes, byte((hi>>24)&0xFF))
				pos = pos + 1
			} else if tokens[pos].Type == TokComma {
				pos = pos + 1
			} else {
				pos = pos + 1
			}
		}
		instr.RawBytes = bytes
		instr.ByteSize = len(bytes)
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".ASCII") {
		if pos < len(tokens) && tokens[pos].Type == TokString {
			bytes := []byte{}
			s := tokens[pos].Text
			k := 0
			for {
				if k >= len(s) {
					break
				}
				bytes = append(bytes, byte(s[k]))
				k = k + 1
			}
			instr.RawBytes = bytes
			instr.ByteSize = len(bytes)
			pos = pos + 1
		}
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".ASCIZ") {
		if pos < len(tokens) && tokens[pos].Type == TokString {
			bytes := []byte{}
			s := tokens[pos].Text
			k := 0
			for {
				if k >= len(s) {
					break
				}
				bytes = append(bytes, byte(s[k]))
				k = k + 1
			}
			bytes = append(bytes, byte(0)) // null terminator
			instr.RawBytes = bytes
			instr.ByteSize = len(bytes)
			pos = pos + 1
		}
		return instr, pos
	}

	if strEqual(instr.Mnemonic, ".SPACE") {
		if pos < len(tokens) && tokens[pos].Type == TokNumber {
			n := parseNumber(tokens[pos].Text)
			bytes := []byte{}
			k := 0
			for {
				if k >= n {
					break
				}
				bytes = append(bytes, byte(0))
				k = k + 1
			}
			instr.RawBytes = bytes
			instr.ByteSize = n
			pos = pos + 1
		}
		return instr, pos
	}

	// Unknown directive, skip to end of line
	for {
		if pos >= len(tokens) {
			break
		}
		if tokens[pos].Type == TokNewline {
			break
		}
		pos = pos + 1
	}
	return instr, pos
}

// Parse converts a token list into parsed instructions
func Parse(tokens []Token) []ParsedInstr {
	instrs := []ParsedInstr{}
	pos := 0
	for {
		if pos >= len(tokens) {
			break
		}
		instr, newPos := parseLine(tokens, pos)
		if instr.Kind != InstrKindNone {
			instrs = append(instrs, instr)
		}
		if newPos <= pos {
			pos = pos + 1 // safety: avoid infinite loop
		} else {
			pos = newPos
		}
	}
	return instrs
}

// --- Label resolution ---

// findLabel looks up a label in the label table
func findLabel(labels []LabelEntry, name string) (int, bool) {
	i := 0
	for {
		if i >= len(labels) {
			break
		}
		if strEqual(labels[i].Name, name) {
			return labels[i].Offset, true
		}
		i = i + 1
	}
	return 0, false
}

// resolveLabels performs two-pass label resolution
func resolveLabels(instrs []ParsedInstr) ([]ParsedInstr, string) {
	// Pass 1: assign offsets and record labels
	labels := []LabelEntry{}
	offset := 0
	i := 0
	for {
		if i >= len(instrs) {
			break
		}
		instr := instrs[i]

		// Handle alignment directive
		if instr.Kind == InstrKindDirective && strEqual(instr.Mnemonic, ".ALIGN") {
			if len(instr.Operands) > 0 {
				alignment := instr.Operands[0].Imm
				if alignment > 0 {
					rem := offset % alignment
					if rem != 0 {
						padding := alignment - rem
						instr.ByteSize = padding
						instr.RawBytes = []byte{}
						k := 0
						for {
							if k >= padding {
								break
							}
							instr.RawBytes = append(instr.RawBytes, byte(0))
							k = k + 1
						}
					}
				}
			}
		}

		// Record label
		if instr.Label != "" {
			labels = append(labels, LabelEntry{Name: instr.Label, Offset: offset})
		}

		instr.Offset = offset
		instrs[i] = instr
		offset = offset + instr.ByteSize
		i = i + 1
	}

	// Pass 2: resolve label references in branch operands
	i = 0
	for {
		if i >= len(instrs) {
			break
		}
		instr := instrs[i]
		if instr.Kind == InstrKindInstr {
			j := 0
			for {
				if j >= len(instr.Operands) {
					break
				}
				if instr.Operands[j].Kind == OpndLabel {
					targetOff, found := findLabel(labels, instr.Operands[j].Label)
					if !found {
						return instrs, "undefined label: " + instr.Operands[j].Label
					}
					// Convert to PC-relative immediate
					pcRel := targetOff - instr.Offset
					instr.Operands[j].Kind = OpndImm
					instr.Operands[j].Imm = pcRel
				}
				j = j + 1
			}
			instrs[i] = instr
		}
		i = i + 1
	}

	return instrs, ""
}

// --- Public API ---

// Assemble parses ARM64 assembly text and returns machine code bytes.
// Returns (code, error) where error is "" on success.
func Assemble(text string) ([]byte, string) {
	tokens := Tokenize(text)
	parsed := Parse(tokens)

	instrs, err := resolveLabels(parsed)
	if err != "" {
		return []byte{}, err
	}

	code := []byte{}
	i := 0
	for {
		if i >= len(instrs) {
			break
		}
		instr := instrs[i]

		if instr.Kind == InstrKindDirective {
			// Emit raw bytes for data directives
			if len(instr.RawBytes) > 0 {
				j := 0
				for {
					if j >= len(instr.RawBytes) {
						break
					}
					code = append(code, instr.RawBytes[j])
					j = j + 1
				}
			}
			i = i + 1
			continue
		}

		if instr.Kind == InstrKindLabel {
			i = i + 1
			continue
		}

		if instr.Kind == InstrKindInstr {
			encoded, encErr := encodeInstruction(instr)
			if encErr != "" {
				return []byte{}, encErr
			}
			j := 0
			for {
				if j >= len(encoded) {
					break
				}
				code = append(code, encoded[j])
				j = j + 1
			}
		}

		i = i + 1
	}

	return code, ""
}

// GetGlobals returns symbol names declared with .global directive.
func GetGlobals(text string) []string {
	tokens := Tokenize(text)
	instrs := Parse(tokens)
	globals := []string{}
	i := 0
	for {
		if i >= len(instrs) {
			break
		}
		instr := instrs[i]
		if instr.Kind == InstrKindDirective {
			if strEqual(instr.Mnemonic, ".GLOBAL") || strEqual(instr.Mnemonic, ".GLOBL") {
				if len(instr.Operands) > 0 {
					globals = append(globals, instr.Operands[0].Label)
				}
			}
		}
		i = i + 1
	}
	return globals
}
