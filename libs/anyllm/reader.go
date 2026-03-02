package anyllm

import "runtime/fs"

func charToString(ch int) string {
	if ch == 0 {
		return ""
	} else if ch == 1 {
		return ""
	} else if ch == 2 {
		return ""
	} else if ch == 3 {
		return ""
	} else if ch == 4 {
		return ""
	} else if ch == 5 {
		return ""
	} else if ch == 6 {
		return ""
	} else if ch == 7 {
		return ""
	} else if ch == 8 {
		return ""
	} else if ch == 9 {
		return "\t"
	} else if ch == 10 {
		return "\n"
	} else if ch == 11 {
		return ""
	} else if ch == 12 {
		return ""
	} else if ch == 13 {
		return "\r"
	} else if ch == 14 {
		return ""
	} else if ch == 15 {
		return ""
	} else if ch == 16 {
		return ""
	} else if ch == 17 {
		return ""
	} else if ch == 18 {
		return ""
	} else if ch == 19 {
		return ""
	} else if ch == 20 {
		return ""
	} else if ch == 21 {
		return ""
	} else if ch == 22 {
		return ""
	} else if ch == 23 {
		return ""
	} else if ch == 24 {
		return ""
	} else if ch == 25 {
		return ""
	} else if ch == 26 {
		return ""
	} else if ch == 27 {
		return ""
	} else if ch == 28 {
		return ""
	} else if ch == 29 {
		return ""
	} else if ch == 30 {
		return ""
	} else if ch == 31 {
		return ""
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

// IntToStringPub converts an int to its string representation
func IntToStringPub(n int) string {
	return intToString(n)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	result := ""
	for n > 0 {
		digit := n % 10
		result = charToString(48+digit) + result
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

func int64ToString(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	result := ""
	for n > 0 {
		digit := n % 10
		digitInt := int(digit)
		result = charToString(48+digitInt) + result
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

func float64ToString(f float64) string {
	if f < 0.0 {
		return "-" + float64ToString(-f)
	}
	intPart := int64(f)
	fracPart := f - float64(intPart)
	result := int64ToString(intPart)
	result += "."
	i := 0
	for i < 6 {
		fracPart = fracPart * 10.0
		d := int(fracPart)
		result += charToString(48 + d)
		fracPart = fracPart - float64(d)
		i = i + 1
	}
	return result
}

// ParseFloat parses a float64 from a string (transpiler-safe, no strconv)
func ParseFloat(s string) float64 {
	if len(s) == 0 {
		return 0.0
	}
	negative := false
	i := 0
	if int(s[0]) == 45 { // '-'
		negative = true
		i = 1
	}
	// Parse integer part
	intPart := 0.0
	for i < len(s) && int(s[i]) >= 48 && int(s[i]) <= 57 {
		intPart = intPart*10.0 + float64(int(s[i])-48)
		i = i + 1
	}
	result := intPart
	// Parse fractional part
	if i < len(s) && int(s[i]) == 46 { // '.'
		i = i + 1
		frac := 0.0
		scale := 0.1
		for i < len(s) && int(s[i]) >= 48 && int(s[i]) <= 57 {
			frac = frac + float64(int(s[i])-48)*scale
			scale = scale / 10.0
			i = i + 1
		}
		result = result + frac
	}
	if negative {
		result = -result
	}
	return result
}

func newReadState(handle int) ReadState {
	rs := ReadState{}
	rs.Handle = handle
	rs.Pos = 0
	rs.Err = ""
	return rs
}

func readBytes(rs ReadState, n int) (ReadState, []byte) {
	if rs.Err != "" {
		empty := []byte{}
		return rs, empty
	}
	rawData, bytesRead, err := fs.Read(rs.Handle, n)
	if err != "" {
		rs.Err = err
		empty := []byte{}
		return rs, empty
	}
	if bytesRead != n {
		rs.Err = "short read"
		empty := []byte{}
		return rs, empty
	}
	rs.Pos = rs.Pos + int64(n)
	result := []byte{}
	i := 0
	for i < n {
		result = append(result, rawData[i])
		i = i + 1
	}
	return rs, result
}

func readUint8(rs ReadState) (ReadState, int) {
	rs2, data := readBytes(rs, 1)
	if rs2.Err != "" {
		return rs2, 0
	}
	return rs2, int(data[0])
}

func readUint32LE(rs ReadState) (ReadState, int) {
	rs2, data := readBytes(rs, 4)
	if rs2.Err != "" {
		return rs2, 0
	}
	b0 := int(data[0])
	b1 := int(data[1])
	b2 := int(data[2])
	b3 := int(data[3])
	val := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
	return rs2, val
}

func readUint64LE(rs ReadState) (ReadState, int64) {
	rs2, data := readBytes(rs, 8)
	if rs2.Err != "" {
		return rs2, 0
	}
	b0 := int64(data[0])
	b1 := int64(data[1])
	b2 := int64(data[2])
	b3 := int64(data[3])
	b4 := int64(data[4])
	b5 := int64(data[5])
	b6 := int64(data[6])
	b7 := int64(data[7])
	val := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24) | (b4 << 32) | (b5 << 40) | (b6 << 48) | (b7 << 56)
	return rs2, val
}

func readFloat32LE(rs ReadState) (ReadState, float64) {
	rs2, data := readBytes(rs, 4)
	if rs2.Err != "" {
		return rs2, 0.0
	}
	b0 := int(data[0])
	b1 := int(data[1])
	b2 := int(data[2])
	b3 := int(data[3])
	bits := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
	sign := (bits >> 31) & 1
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF
	if exp == 0 && frac == 0 {
		if sign == 1 {
			return rs2, -0.0
		}
		return rs2, 0.0
	}
	e := float64(exp - 127)
	m := 1.0 + float64(frac)/8388608.0
	result := m * pow2(e)
	if sign == 1 {
		result = -result
	}
	return rs2, result
}

func readFloat64LE(rs ReadState) (ReadState, float64) {
	rs2, data := readBytes(rs, 8)
	if rs2.Err != "" {
		return rs2, 0.0
	}
	b0 := int64(data[0])
	b1 := int64(data[1])
	b2 := int64(data[2])
	b3 := int64(data[3])
	b4 := int64(data[4])
	b5 := int64(data[5])
	b6 := int64(data[6])
	b7 := int64(data[7])
	bits := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24) | (b4 << 32) | (b5 << 40) | (b6 << 48) | (b7 << 56)
	sign := int((bits >> 63) & 1)
	exp := int((bits >> 52) & 0x7FF)
	frac := bits & 0xFFFFFFFFFFFFF
	if exp == 0 && frac == 0 {
		if sign == 1 {
			return rs2, -0.0
		}
		return rs2, 0.0
	}
	e := float64(exp - 1023)
	m := 1.0 + float64(frac)/4503599627370496.0
	result := m * pow2(e)
	if sign == 1 {
		result = -result
	}
	return rs2, result
}

func pow2(e float64) float64 {
	if e == 0.0 {
		return 1.0
	}
	if e < 0.0 {
		return 1.0 / pow2(-e)
	}
	intExp := int(e)
	result := 1.0
	i := 0
	for i < intExp {
		result = result * 2.0
		i = i + 1
	}
	return result
}

func readGGUFString(rs ReadState) (ReadState, string) {
	rs2, length := readUint64LE(rs)
	if rs2.Err != "" {
		return rs2, ""
	}
	n := int(length)
	rs3, data := readBytes(rs2, n)
	if rs3.Err != "" {
		return rs3, ""
	}
	result := ""
	i := 0
	for i < n {
		ch := int(data[i])
		result += charToString(ch)
		i = i + 1
	}
	return rs3, result
}

func skipBytes(rs ReadState, n int64) ReadState {
	if rs.Err != "" {
		return rs
	}
	newPos := rs.Pos + n
	_, err := fs.Seek(rs.Handle, newPos, fs.SeekStart)
	if err != "" {
		rs.Err = err
		return rs
	}
	rs.Pos = newPos
	return rs
}
