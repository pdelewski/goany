package arm64asm

// emitLE32 encodes a 32-bit instruction word as 4 little-endian bytes
func emitLE32(word int) []byte {
	buf := []byte{}
	buf = append(buf, byte(word&0xFF))
	buf = append(buf, byte((word>>8)&0xFF))
	buf = append(buf, byte((word>>16)&0xFF))
	buf = append(buf, byte((word>>24)&0xFF))
	return buf
}

// --- Encoding helpers for specific instruction formats ---

// encodeAddSubImm encodes ADD/SUB immediate
// sf=1 for 64-bit, op=0 for ADD, op=1 for SUB, S=1 for ADDS/SUBS
func encodeAddSubImm(sf int, op int, s int, shift int, imm12 int, rn int, rd int) []byte {
	// [31]sf [30]op [29]S [28:24]10001 [23:22]sh [21:10]imm12 [9:5]Rn [4:0]Rd
	word := (sf << 31) | (op << 30) | (s << 29) | (0x11 << 24) | (shift << 22) | ((imm12 & 0xFFF) << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeAddSubReg encodes ADD/SUB shifted register
func encodeAddSubReg(sf int, op int, s int, shiftType int, rm int, imm6 int, rn int, rd int) []byte {
	// [31]sf [30]op [29]S [28:24]01011 [23:22]shift [21]0 [20:16]Rm [15:10]imm6 [9:5]Rn [4:0]Rd
	word := (sf << 31) | (op << 30) | (s << 29) | (0x0B << 24) | (shiftType << 22) | (rm << 16) | ((imm6 & 0x3F) << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeLogicalReg encodes logical (shifted register) instructions
func encodeLogicalReg(sf int, opc int, n int, shiftType int, rm int, imm6 int, rn int, rd int) []byte {
	// [31]sf [30:29]opc [28:24]01010 [23:22]shift [21]N [20:16]Rm [15:10]imm6 [9:5]Rn [4:0]Rd
	word := (sf << 31) | (opc << 29) | (0x0A << 24) | (shiftType << 22) | (n << 21) | (rm << 16) | ((imm6 & 0x3F) << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeMoveWide encodes MOVZ/MOVK/MOVN
func encodeMoveWide(sf int, opc int, hw int, imm16 int, rd int) []byte {
	// [31]sf [30:29]opc [28:23]100101 [22:21]hw [20:5]imm16 [4:0]Rd
	word := (sf << 31) | (opc << 29) | (0x25 << 23) | (hw << 21) | ((imm16 & 0xFFFF) << 5) | rd
	return emitLE32(word)
}

// encodeBranch encodes B or BL (unconditional branch)
func encodeBranch(op int, imm26 int) []byte {
	// [31]op [30:26]00101 [25:0]imm26
	word := (op << 31) | (0x05 << 26) | (imm26 & 0x3FFFFFF)
	return emitLE32(word)
}

// encodeBranchCond encodes B.cond
func encodeBranchCond(imm19 int, cond int) []byte {
	// [31:24]01010100 [23:5]imm19 [4]0 [3:0]cond
	word := (0x54 << 24) | ((imm19 & 0x7FFFF) << 5) | cond
	return emitLE32(word)
}

// encodeBranchReg encodes BR, BLR, RET
func encodeBranchReg(opc int, rn int) []byte {
	// [31:25]1101011 [24:21]opc [20:16]11111 [15:10]000000 [9:5]Rn [4:0]00000
	word := (0x6B << 25) | (opc << 21) | (0x1F << 16) | (rn << 5)
	return emitLE32(word)
}

// encodeLdrStrUnsigned encodes LDR/STR with unsigned offset
func encodeLdrStrUnsigned(size int, v int, opc int, imm12 int, rn int, rt int) []byte {
	// [31:30]size [29:27]111 [26]V [25:24]01 [23:22]opc [21:10]imm12 [9:5]Rn [4:0]Rt
	word := (size << 30) | (0x07 << 27) | (v << 26) | (0x01 << 24) | (opc << 22) | ((imm12 & 0xFFF) << 10) | (rn << 5) | rt
	return emitLE32(word)
}

// encodeLdrStrPrePost encodes LDR/STR with pre/post-index
func encodeLdrStrPrePost(size int, v int, opc int, imm9 int, mode int, rn int, rt int) []byte {
	// [31:30]size [29:27]111 [26]V [25:24]00 [23:22]opc [21]0 [20:12]imm9 [11:10]mode [9:5]Rn [4:0]Rt
	word := (size << 30) | (0x07 << 27) | (v << 26) | (opc << 22) | ((imm9 & 0x1FF) << 12) | (mode << 10) | (rn << 5) | rt
	return emitLE32(word)
}

// encodeLdpStp encodes LDP/STP
func encodeLdpStp(opc int, v int, mode int, l int, imm7 int, rt2 int, rn int, rt1 int) []byte {
	// [31:30]opc [29:27]101 [26]V [25:23]mode [22]L [21:15]imm7 [14:10]Rt2 [9:5]Rn [4:0]Rt1
	word := (opc << 30) | (0x05 << 27) | (v << 26) | (mode << 23) | (l << 22) | ((imm7 & 0x7F) << 15) | (rt2 << 10) | (rn << 5) | rt1
	return emitLE32(word)
}

// encodeMul encodes MUL (MADD with Ra=XZR)
func encodeMul(sf int, rm int, rn int, rd int) []byte {
	// MADD Rd, Rn, Rm, XZR: [31]sf 00 11011 000 [20:16]Rm 0 [14:10]11111 [9:5]Rn [4:0]Rd
	word := (sf << 31) | (0x1B << 24) | (rm << 16) | (0x1F << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeDiv encodes SDIV/UDIV
func encodeDiv(sf int, isSignedDiv int, rm int, rn int, rd int) []byte {
	// [31]sf 0 0 11010110 [20:16]Rm 00001 [10]isSignedDiv [9:5]Rn [4:0]Rd
	word := (sf << 31) | (0xD6 << 21) | (rm << 16) | (0x02 << 11) | (isSignedDiv << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeShiftReg encodes LSLV/LSRV/ASRV (register-register shift)
func encodeShiftReg(sf int, rm int, op2 int, rn int, rd int) []byte {
	// [31]sf 0 0 11010110 [20:16]Rm 0010 [11:10]op2 [9:5]Rn [4:0]Rd
	word := (sf << 31) | (0xD6 << 21) | (rm << 16) | (0x02 << 12) | (op2 << 10) | (rn << 5) | rd
	return emitLE32(word)
}

// encodeCBZ encodes CBZ/CBNZ
func encodeCBZ(sf int, op int, imm19 int, rt int) []byte {
	// [31]sf 011010 [24]op [23:5]imm19 [4:0]Rt
	word := (sf << 31) | (0x1A << 25) | (op << 24) | ((imm19 & 0x7FFFF) << 5) | rt
	return emitLE32(word)
}

// --- Main encoding dispatcher ---

// encodeInstruction encodes a single parsed instruction into bytes
func encodeInstruction(instr ParsedInstr) ([]byte, string) {
	ops := instr.Operands
	condCode := instr.CondCode
	nops := len(ops)

	// --- System instructions ---
	if strEqual(instr.Mnemonic, "NOP") {
		// NOP = 0xD503201F, built from smaller parts to avoid C# int overflow
		return emitLE32((0xD5 << 24) | (0x03 << 16) | (0x20 << 8) | 0x1F), ""
	}

	if strEqual(instr.Mnemonic, "SVC") {
		imm := 0
		if nops > 0 && ops[0].Kind == OpndImm {
			imm = ops[0].Imm
		}
		// SVC base = 0xD4000001, built from parts
		word := (0xD4 << 24) | 0x01 | ((imm & 0xFFFF) << 5)
		return emitLE32(word), ""
	}

	if strEqual(instr.Mnemonic, "BRK") {
		imm := 0
		if nops > 0 && ops[0].Kind == OpndImm {
			imm = ops[0].Imm
		}
		// BRK base = 0xD4200000, built from parts
		word := (0xD4 << 24) | (0x20 << 16) | ((imm & 0xFFFF) << 5)
		return emitLE32(word), ""
	}

	// --- Branches ---
	if strEqual(instr.Mnemonic, "B") {
		if nops > 0 {
			if ops[0].Kind == OpndImm {
				pcRel := ops[0].Imm
				imm26 := (pcRel >> 2) & 0x3FFFFFF
				return encodeBranch(0, imm26), ""
			}
		}
		return []byte{}, "B: expected label or immediate operand"
	}

	if strEqual(instr.Mnemonic, "BL") {
		if nops > 0 {
			if ops[0].Kind == OpndImm {
				pcRel := ops[0].Imm
				imm26 := (pcRel >> 2) & 0x3FFFFFF
				return encodeBranch(1, imm26), ""
			}
		}
		return []byte{}, "BL: expected label or immediate operand"
	}

	if strEqual(instr.Mnemonic, "B.COND") {
		if nops > 0 && ops[0].Kind == OpndImm {
			pcRel := ops[0].Imm
			imm19 := (pcRel >> 2) & 0x7FFFF
			return encodeBranchCond(imm19, condCode), ""
		}
		return []byte{}, "B.cond: expected label or immediate operand"
	}

	if strEqual(instr.Mnemonic, "BR") {
		if nops > 0 && ops[0].Kind == OpndReg {
			return encodeBranchReg(0, ops[0].Reg), ""
		}
		return []byte{}, "BR: expected register operand"
	}

	if strEqual(instr.Mnemonic, "BLR") {
		if nops > 0 && ops[0].Kind == OpndReg {
			return encodeBranchReg(1, ops[0].Reg), ""
		}
		return []byte{}, "BLR: expected register operand"
	}

	if strEqual(instr.Mnemonic, "RET") {
		rn := 30 // default X30
		if nops > 0 && ops[0].Kind == OpndReg {
			rn = ops[0].Reg
		}
		return encodeBranchReg(2, rn), ""
	}

	// --- CBZ/CBNZ ---
	if strEqual(instr.Mnemonic, "CBZ") {
		if nops >= 2 && ops[0].Kind == OpndReg && ops[1].Kind == OpndImm {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			pcRel := ops[1].Imm
			imm19 := (pcRel >> 2) & 0x7FFFF
			return encodeCBZ(sf, 0, imm19, ops[0].Reg), ""
		}
		return []byte{}, "CBZ: expected Rt, label"
	}

	if strEqual(instr.Mnemonic, "CBNZ") {
		if nops >= 2 && ops[0].Kind == OpndReg && ops[1].Kind == OpndImm {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			pcRel := ops[1].Imm
			imm19 := (pcRel >> 2) & 0x7FFFF
			return encodeCBZ(sf, 1, imm19, ops[0].Reg), ""
		}
		return []byte{}, "CBNZ: expected Rt, label"
	}

	// --- MOV aliases ---
	if strEqual(instr.Mnemonic, "MOV") {
		return encodeMov(ops, nops)
	}

	// --- MOVZ, MOVK, MOVN ---
	if strEqual(instr.Mnemonic, "MOVZ") {
		return encodeMoveWideInstr(0x02, ops, nops) // opc=10
	}
	if strEqual(instr.Mnemonic, "MOVK") {
		return encodeMoveWideInstr(0x03, ops, nops) // opc=11
	}
	if strEqual(instr.Mnemonic, "MOVN") {
		return encodeMoveWideInstr(0x00, ops, nops) // opc=00
	}

	// --- CMP alias (SUBS with ZR dest) ---
	if strEqual(instr.Mnemonic, "CMP") {
		return encodeCmp(ops, nops)
	}

	// --- CMN alias (ADDS with ZR dest) ---
	if strEqual(instr.Mnemonic, "CMN") {
		return encodeCmn(ops, nops)
	}

	// --- TST alias (ANDS with ZR dest) ---
	if strEqual(instr.Mnemonic, "TST") {
		if nops >= 2 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			shiftType := 0
			imm6 := 0
			if nops >= 3 && ops[2].Kind == OpndShift {
				shiftType = ops[2].ShiftType
				imm6 = ops[2].ShiftAmt
			}
			// ANDS with Rd=XZR(31): opc=11, N=0
			return encodeLogicalReg(sf, 3, 0, shiftType, ops[1].Reg, imm6, ops[0].Reg, 31), ""
		}
		return []byte{}, "TST: expected Rn, Rm"
	}

	// --- NEG alias (SUB from ZR) ---
	if strEqual(instr.Mnemonic, "NEG") {
		if nops >= 2 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			return encodeAddSubReg(sf, 1, 0, 0, ops[1].Reg, 0, 31, ops[0].Reg), ""
		}
		return []byte{}, "NEG: expected Rd, Rm"
	}

	// --- MVN alias (ORN from ZR) ---
	if strEqual(instr.Mnemonic, "MVN") {
		if nops >= 2 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			// ORN: opc=01, N=1
			return encodeLogicalReg(sf, 1, 1, 0, ops[1].Reg, 0, 31, ops[0].Reg), ""
		}
		return []byte{}, "MVN: expected Rd, Rm"
	}

	// --- ADD/ADDS ---
	if strEqual(instr.Mnemonic, "ADD") || strEqual(instr.Mnemonic, "ADDS") {
		s := 0
		if strEqual(instr.Mnemonic, "ADDS") {
			s = 1
		}
		return encodeAddSub(0, s, ops, nops)
	}

	// --- SUB/SUBS ---
	if strEqual(instr.Mnemonic, "SUB") || strEqual(instr.Mnemonic, "SUBS") {
		s := 0
		if strEqual(instr.Mnemonic, "SUBS") {
			s = 1
		}
		return encodeAddSub(1, s, ops, nops)
	}

	// --- Logical register: AND, ORR, EOR, ANDS ---
	if strEqual(instr.Mnemonic, "AND") {
		return encodeLogical(0, ops, nops) // opc=00
	}
	if strEqual(instr.Mnemonic, "ORR") {
		return encodeLogical(1, ops, nops) // opc=01
	}
	if strEqual(instr.Mnemonic, "EOR") {
		return encodeLogical(2, ops, nops) // opc=10
	}
	if strEqual(instr.Mnemonic, "ANDS") {
		return encodeLogical(3, ops, nops) // opc=11
	}

	// --- MUL ---
	if strEqual(instr.Mnemonic, "MUL") {
		if nops >= 3 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg && ops[2].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			return encodeMul(sf, ops[2].Reg, ops[1].Reg, ops[0].Reg), ""
		}
		return []byte{}, "MUL: expected Rd, Rn, Rm"
	}

	// --- SDIV ---
	if strEqual(instr.Mnemonic, "SDIV") {
		if nops >= 3 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg && ops[2].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			return encodeDiv(sf, 1, ops[2].Reg, ops[1].Reg, ops[0].Reg), ""
		}
		return []byte{}, "SDIV: expected Rd, Rn, Rm"
	}

	// --- UDIV ---
	if strEqual(instr.Mnemonic, "UDIV") {
		if nops >= 3 && ops[0].Kind == OpndReg && ops[1].Kind == OpndReg && ops[2].Kind == OpndReg {
			sf := 1
			if ops[0].Is32 {
				sf = 0
			}
			return encodeDiv(sf, 0, ops[2].Reg, ops[1].Reg, ops[0].Reg), ""
		}
		return []byte{}, "UDIV: expected Rd, Rn, Rm"
	}

	// --- Shift instructions (register-register): LSL, LSR, ASR ---
	if strEqual(instr.Mnemonic, "LSL") {
		return encodeShiftInstr(0, ops, nops) // op2=00
	}
	if strEqual(instr.Mnemonic, "LSR") {
		return encodeShiftInstr(1, ops, nops) // op2=01
	}
	if strEqual(instr.Mnemonic, "ASR") {
		return encodeShiftInstr(2, ops, nops) // op2=10
	}

	// --- Load/Store ---
	if strEqual(instr.Mnemonic, "LDR") {
		return encodeLdrStr(1, 0, ops, nops) // opc=01 for LDR
	}
	if strEqual(instr.Mnemonic, "STR") {
		return encodeLdrStr(0, 0, ops, nops) // opc=00 for STR
	}
	if strEqual(instr.Mnemonic, "LDRB") {
		return encodeLdrStrBH(0, 1, ops, nops) // size=0, opc=01 for LDRB
	}
	if strEqual(instr.Mnemonic, "STRB") {
		return encodeLdrStrBH(0, 0, ops, nops) // size=0, opc=00 for STRB
	}
	if strEqual(instr.Mnemonic, "LDRH") {
		return encodeLdrStrBH(1, 1, ops, nops) // size=1, opc=01 for LDRH
	}
	if strEqual(instr.Mnemonic, "STRH") {
		return encodeLdrStrBH(1, 0, ops, nops) // size=1, opc=00 for STRH
	}

	// --- LDP/STP ---
	if strEqual(instr.Mnemonic, "LDP") {
		return encodeLdpStpInstr(1, ops, nops) // L=1 for LDP
	}
	if strEqual(instr.Mnemonic, "STP") {
		return encodeLdpStpInstr(0, ops, nops) // L=0 for STP
	}

	return []byte{}, "unknown instruction: " + instr.Mnemonic
}

// --- Helper encoding functions ---

// encodeMov handles MOV Rd, Rm or MOV Rd, #imm
func encodeMov(ops []Operand, nops int) ([]byte, string) {
	if nops < 2 {
		return []byte{}, "MOV: expected 2 operands"
	}
	if ops[0].Kind != OpndReg {
		return []byte{}, "MOV: expected register destination"
	}

	sf := 1
	if ops[0].Is32 {
		sf = 0
	}

	// MOV Rd, Rm → ORR Rd, XZR, Rm
	if ops[1].Kind == OpndReg {
		// opc=01 (ORR), N=0, shift=LSL, imm6=0
		return encodeLogicalReg(sf, 1, 0, 0, ops[1].Reg, 0, 31, ops[0].Reg), ""
	}

	// MOV Rd, #imm
	if ops[1].Kind == OpndImm {
		imm := ops[1].Imm
		if imm >= 0 && imm <= 65535 {
			// MOVZ
			return encodeMoveWide(sf, 2, 0, imm, ops[0].Reg), ""
		}
		if imm < 0 && imm >= -65536 {
			// MOVN: encode ~imm
			notImm := ^imm
			if sf == 0 {
				notImm = notImm & 0xFFFF
			}
			return encodeMoveWide(sf, 0, 0, notImm&0xFFFF, ops[0].Reg), ""
		}
		// For larger values, try MOVZ with shift
		return []byte{}, "MOV: immediate value out of range"
	}

	return []byte{}, "MOV: unexpected operand type"
}

// encodeMoveWideInstr encodes MOVZ/MOVK/MOVN instructions
func encodeMoveWideInstr(opc int, ops []Operand, nops int) ([]byte, string) {
	if nops < 2 || ops[0].Kind != OpndReg || ops[1].Kind != OpndImm {
		return []byte{}, "MOVx: expected Rd, #imm"
	}
	sf := 1
	if ops[0].Is32 {
		sf = 0
	}
	hw := 0
	// Check for LSL shift on the immediate operand
	if ops[1].ShiftAmt > 0 {
		hw = ops[1].ShiftAmt / 16
	}
	// Also check if there's a 3rd operand that's a shift
	if nops >= 3 && ops[2].Kind == OpndShift && ops[2].ShiftType == ShiftLSL {
		hw = ops[2].ShiftAmt / 16
	}
	return encodeMoveWide(sf, opc, hw, ops[1].Imm&0xFFFF, ops[0].Reg), ""
}

// encodeAddSub encodes ADD/SUB/ADDS/SUBS (immediate or register)
func encodeAddSub(op int, s int, ops []Operand, nops int) ([]byte, string) {
	if nops < 3 {
		return []byte{}, "ADD/SUB: expected 3 operands"
	}
	if ops[0].Kind != OpndReg || ops[1].Kind != OpndReg {
		return []byte{}, "ADD/SUB: expected Rd, Rn, ..."
	}

	sf := 1
	if ops[0].Is32 {
		sf = 0
	}

	// Immediate form
	if ops[2].Kind == OpndImm {
		shift := 0
		// Check for LSL #12
		if nops >= 4 && ops[3].Kind == OpndShift && ops[3].ShiftType == ShiftLSL && ops[3].ShiftAmt == 12 {
			shift = 1
		}
		imm := ops[2].Imm
		if imm < 0 {
			// Flip ADD<->SUB for negative immediates
			imm = -imm
			if op == 0 {
				op = 1
			} else {
				op = 0
			}
		}
		return encodeAddSubImm(sf, op, s, shift, imm&0xFFF, ops[1].Reg, ops[0].Reg), ""
	}

	// Register form
	if ops[2].Kind == OpndReg {
		shiftType := 0
		imm6 := 0
		// Check for optional shift
		if ops[2].ShiftAmt > 0 {
			shiftType = ops[2].ShiftType
			imm6 = ops[2].ShiftAmt
		}
		if nops >= 4 && ops[3].Kind == OpndShift {
			shiftType = ops[3].ShiftType
			imm6 = ops[3].ShiftAmt
		}
		return encodeAddSubReg(sf, op, s, shiftType, ops[2].Reg, imm6, ops[1].Reg, ops[0].Reg), ""
	}

	return []byte{}, "ADD/SUB: unexpected third operand"
}

// encodeCmp encodes CMP (alias for SUBS with ZR dest)
func encodeCmp(ops []Operand, nops int) ([]byte, string) {
	if nops < 2 || ops[0].Kind != OpndReg {
		return []byte{}, "CMP: expected Rn, operand"
	}

	sf := 1
	if ops[0].Is32 {
		sf = 0
	}

	// CMP Rn, #imm → SUBS XZR, Rn, #imm
	if ops[1].Kind == OpndImm {
		imm := ops[1].Imm
		op := 1 // SUB
		if imm < 0 {
			imm = -imm
			op = 0 // ADD (CMN)
		}
		shift := 0
		if nops >= 3 && ops[2].Kind == OpndShift && ops[2].ShiftType == ShiftLSL && ops[2].ShiftAmt == 12 {
			shift = 1
		}
		return encodeAddSubImm(sf, op, 1, shift, imm&0xFFF, ops[0].Reg, 31), ""
	}

	// CMP Rn, Rm → SUBS XZR, Rn, Rm
	if ops[1].Kind == OpndReg {
		shiftType := 0
		imm6 := 0
		if ops[1].ShiftAmt > 0 {
			shiftType = ops[1].ShiftType
			imm6 = ops[1].ShiftAmt
		}
		if nops >= 3 && ops[2].Kind == OpndShift {
			shiftType = ops[2].ShiftType
			imm6 = ops[2].ShiftAmt
		}
		return encodeAddSubReg(sf, 1, 1, shiftType, ops[1].Reg, imm6, ops[0].Reg, 31), ""
	}

	return []byte{}, "CMP: unexpected operand"
}

// encodeCmn encodes CMN (alias for ADDS with ZR dest)
func encodeCmn(ops []Operand, nops int) ([]byte, string) {
	if nops < 2 || ops[0].Kind != OpndReg {
		return []byte{}, "CMN: expected Rn, operand"
	}

	sf := 1
	if ops[0].Is32 {
		sf = 0
	}

	if ops[1].Kind == OpndImm {
		return encodeAddSubImm(sf, 0, 1, 0, ops[1].Imm&0xFFF, ops[0].Reg, 31), ""
	}

	if ops[1].Kind == OpndReg {
		return encodeAddSubReg(sf, 0, 1, 0, ops[1].Reg, 0, ops[0].Reg, 31), ""
	}

	return []byte{}, "CMN: unexpected operand"
}

// encodeLogical encodes AND/ORR/EOR/ANDS (register form only)
func encodeLogical(opc int, ops []Operand, nops int) ([]byte, string) {
	if nops < 3 || ops[0].Kind != OpndReg || ops[1].Kind != OpndReg || ops[2].Kind != OpndReg {
		return []byte{}, "AND/ORR/EOR: expected Rd, Rn, Rm"
	}

	sf := 1
	if ops[0].Is32 {
		sf = 0
	}

	shiftType := 0
	imm6 := 0
	if ops[2].ShiftAmt > 0 {
		shiftType = ops[2].ShiftType
		imm6 = ops[2].ShiftAmt
	}
	if nops >= 4 && ops[3].Kind == OpndShift {
		shiftType = ops[3].ShiftType
		imm6 = ops[3].ShiftAmt
	}

	// N=0 for AND/ORR/EOR/ANDS (register, non-inverted)
	return encodeLogicalReg(sf, opc, 0, shiftType, ops[2].Reg, imm6, ops[1].Reg, ops[0].Reg), ""
}

// encodeShiftInstr encodes LSL/LSR/ASR Rd, Rn, Rm (register shift)
func encodeShiftInstr(op2 int, ops []Operand, nops int) ([]byte, string) {
	if nops < 3 || ops[0].Kind != OpndReg || ops[1].Kind != OpndReg || ops[2].Kind != OpndReg {
		return []byte{}, "LSL/LSR/ASR: expected Rd, Rn, Rm"
	}
	sf := 1
	if ops[0].Is32 {
		sf = 0
	}
	return encodeShiftReg(sf, ops[2].Reg, op2, ops[1].Reg, ops[0].Reg), ""
}

// encodeLdrStr encodes LDR/STR (32-bit or 64-bit based on register width)
func encodeLdrStr(isLoad int, unused int, ops []Operand, nops int) ([]byte, string) {
	if nops < 2 || ops[0].Kind != OpndReg {
		return []byte{}, "LDR/STR: expected Rt, [Xn, ...]"
	}

	rt := ops[0].Reg
	is32 := ops[0].Is32
	size := 3 // 64-bit (X register)
	if is32 {
		size = 2 // 32-bit (W register)
	}

	opc := isLoad // 0=STR, 1=LDR

	memOp := ops[1]

	// Unsigned offset
	if memOp.Kind == OpndMem {
		// Scale immediate by access size
		imm := memOp.Imm
		scaledImm := imm
		if size == 3 {
			scaledImm = imm / 8 // 64-bit: scale by 8
		} else if size == 2 {
			scaledImm = imm / 4 // 32-bit: scale by 4
		}
		return encodeLdrStrUnsigned(size, 0, opc, scaledImm&0xFFF, memOp.Reg, rt), ""
	}

	// Pre-index
	if memOp.Kind == OpndMemPre {
		return encodeLdrStrPrePost(size, 0, opc, memOp.Imm&0x1FF, 3, memOp.Reg, rt), ""
	}

	// Post-index
	if memOp.Kind == OpndMemPost {
		return encodeLdrStrPrePost(size, 0, opc, memOp.Imm&0x1FF, 1, memOp.Reg, rt), ""
	}

	return []byte{}, "LDR/STR: unsupported addressing mode"
}

// encodeLdrStrBH encodes LDRB/STRB/LDRH/STRH
func encodeLdrStrBH(size int, isLoad int, ops []Operand, nops int) ([]byte, string) {
	if nops < 2 || ops[0].Kind != OpndReg {
		return []byte{}, "LDRB/STRB/LDRH/STRH: expected Rt, [Xn, ...]"
	}

	rt := ops[0].Reg
	opc := isLoad

	memOp := ops[1]

	// Unsigned offset (no scaling needed for byte, scale by 2 for halfword)
	if memOp.Kind == OpndMem {
		imm := memOp.Imm
		scaledImm := imm
		if size == 1 {
			scaledImm = imm / 2 // halfword: scale by 2
		}
		return encodeLdrStrUnsigned(size, 0, opc, scaledImm&0xFFF, memOp.Reg, rt), ""
	}

	// Pre-index
	if memOp.Kind == OpndMemPre {
		return encodeLdrStrPrePost(size, 0, opc, memOp.Imm&0x1FF, 3, memOp.Reg, rt), ""
	}

	// Post-index
	if memOp.Kind == OpndMemPost {
		return encodeLdrStrPrePost(size, 0, opc, memOp.Imm&0x1FF, 1, memOp.Reg, rt), ""
	}

	return []byte{}, "LDRB/STRB: unsupported addressing mode"
}

// encodeLdpStpInstr encodes LDP/STP
func encodeLdpStpInstr(l int, ops []Operand, nops int) ([]byte, string) {
	if nops < 3 || ops[0].Kind != OpndReg || ops[1].Kind != OpndReg {
		return []byte{}, "LDP/STP: expected Rt1, Rt2, [Xn, ...]"
	}

	rt1 := ops[0].Reg
	rt2 := ops[1].Reg
	is32 := ops[0].Is32

	opc := 2 // 64-bit (10)
	scale := 8
	if is32 {
		opc = 0 // 32-bit (00)
		scale = 4
	}

	memOp := ops[2]

	// Signed offset
	if memOp.Kind == OpndMem {
		imm7 := (memOp.Imm / scale) & 0x7F
		// mode=010 for signed offset
		return encodeLdpStp(opc, 0, 2, l, imm7, rt2, memOp.Reg, rt1), ""
	}

	// Pre-index
	if memOp.Kind == OpndMemPre {
		imm7 := (memOp.Imm / scale) & 0x7F
		// mode=011 for pre-index
		return encodeLdpStp(opc, 0, 3, l, imm7, rt2, memOp.Reg, rt1), ""
	}

	// Post-index
	if memOp.Kind == OpndMemPost {
		imm7 := (memOp.Imm / scale) & 0x7F
		// mode=001 for post-index
		return encodeLdpStp(opc, 0, 1, l, imm7, rt2, memOp.Reg, rt1), ""
	}

	return []byte{}, "LDP/STP: unsupported addressing mode"
}
