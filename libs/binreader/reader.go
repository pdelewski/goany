package binreader

import "runtime/fs"

// ReadState tracks the state of binary reading
type ReadState struct {
	Handle int
	Pos    int64
	Err    string
}

// NewReadState creates a new ReadState for the given file handle
func NewReadState(handle int) ReadState {
	rs := ReadState{}
	rs.Handle = handle
	rs.Pos = 0
	rs.Err = ""
	return rs
}

// ReadBytes reads n bytes from the current position
func ReadBytes(rs ReadState, n int) (ReadState, []byte) {
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

// ReadUint8 reads a single byte as an unsigned int
func ReadUint8(rs ReadState) (ReadState, int) {
	rs2, data := ReadBytes(rs, 1)
	if rs2.Err != "" {
		return rs2, 0
	}
	return rs2, int(data[0])
}

// ReadUint16LE reads a 2-byte little-endian unsigned int
func ReadUint16LE(rs ReadState) (ReadState, int) {
	rs2, data := ReadBytes(rs, 2)
	if rs2.Err != "" {
		return rs2, 0
	}
	b0 := int(data[0])
	b1 := int(data[1])
	val := b0 | (b1 << 8)
	return rs2, val
}

// ReadUint32BE reads a 4-byte big-endian unsigned int
func ReadUint32BE(rs ReadState) (ReadState, int) {
	rs2, data := ReadBytes(rs, 4)
	if rs2.Err != "" {
		return rs2, 0
	}
	b0 := int(data[0])
	b1 := int(data[1])
	b2 := int(data[2])
	b3 := int(data[3])
	val := (b0 << 24) | (b1 << 16) | (b2 << 8) | b3
	return rs2, val
}

// ReadUint32LE reads a 4-byte little-endian unsigned int
func ReadUint32LE(rs ReadState) (ReadState, int) {
	rs2, data := ReadBytes(rs, 4)
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

// ReadUint64LE reads an 8-byte little-endian unsigned int as int64
func ReadUint64LE(rs ReadState) (ReadState, int64) {
	rs2, data := ReadBytes(rs, 8)
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

// ReadFloat32LE reads a 4-byte little-endian IEEE 754 float as float64
func ReadFloat32LE(rs ReadState) (ReadState, float64) {
	rs2, data := ReadBytes(rs, 4)
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
	result := m * Pow2(e)
	if sign == 1 {
		result = -result
	}
	return rs2, result
}

// ReadFloat64LE reads an 8-byte little-endian IEEE 754 double as float64
func ReadFloat64LE(rs ReadState) (ReadState, float64) {
	rs2, data := ReadBytes(rs, 8)
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
	result := m * Pow2(e)
	if sign == 1 {
		result = -result
	}
	return rs2, result
}

// SkipBytes skips n bytes by seeking forward
func SkipBytes(rs ReadState, n int64) ReadState {
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

// Pow2 computes 2^e for integer exponents (used in float decoding)
func Pow2(e float64) float64 {
	if e == 0.0 {
		return 1.0
	}
	if e < 0.0 {
		return 1.0 / Pow2(-e)
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

// ReadFixedString reads n bytes and returns a string, stopping at the first null byte
func ReadFixedString(rs ReadState, n int) (ReadState, string) {
	rs2, data := ReadBytes(rs, n)
	if rs2.Err != "" {
		return rs2, ""
	}
	result := ""
	i := 0
	for i < n {
		ch := int(data[i])
		if ch == 0 {
			return rs2, result
		}
		result += CharToString(ch)
		i = i + 1
	}
	return rs2, result
}

// ExtractString extracts a null-terminated string from a byte slice at the given offset
func ExtractString(data []byte, offset int) string {
	result := ""
	i := offset
	for i < len(data) {
		ch := int(data[i])
		if ch == 0 {
			return result
		}
		result += CharToString(ch)
		i = i + 1
	}
	return result
}
