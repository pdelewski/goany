package anyllm

import "libs/binreader"

func toBR(rs ReadState) binreader.ReadState {
	br := binreader.ReadState{}
	br.Handle = rs.Handle
	br.Pos = rs.Pos
	br.Err = rs.Err
	return br
}

func fromBR(br binreader.ReadState) ReadState {
	rs := ReadState{}
	rs.Handle = br.Handle
	rs.Pos = br.Pos
	rs.Err = br.Err
	return rs
}

func charToString(ch int) string {
	return binreader.CharToString(ch)
}

// IntToStringPub converts an int to its string representation
func IntToStringPub(n int) string {
	return binreader.IntToString(n)
}

func intToString(n int) string {
	return binreader.IntToString(n)
}

func int64ToString(n int64) string {
	return binreader.Int64ToString(n)
}

func float64ToString(f float64) string {
	return binreader.Float64ToString(f)
}

// ParseFloat parses a float64 from a string (transpiler-safe, no strconv)
func ParseFloat(s string) float64 {
	return binreader.ParseFloat(s)
}

func newReadState(handle int) ReadState {
	br := binreader.NewReadState(handle)
	return fromBR(br)
}

func readBytes(rs ReadState, n int) (ReadState, []byte) {
	br2, data := binreader.ReadBytes(toBR(rs), n)
	return fromBR(br2), data
}

func readUint8(rs ReadState) (ReadState, int) {
	br2, val := binreader.ReadUint8(toBR(rs))
	return fromBR(br2), val
}

func readUint32LE(rs ReadState) (ReadState, int) {
	br2, val := binreader.ReadUint32LE(toBR(rs))
	return fromBR(br2), val
}

func readUint64LE(rs ReadState) (ReadState, int64) {
	br2, val := binreader.ReadUint64LE(toBR(rs))
	return fromBR(br2), val
}

func readFloat32LE(rs ReadState) (ReadState, float64) {
	br2, val := binreader.ReadFloat32LE(toBR(rs))
	return fromBR(br2), val
}

func readFloat64LE(rs ReadState) (ReadState, float64) {
	br2, val := binreader.ReadFloat64LE(toBR(rs))
	return fromBR(br2), val
}

func pow2(e float64) float64 {
	return binreader.Pow2(e)
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
	br2 := binreader.SkipBytes(toBR(rs), n)
	return fromBR(br2)
}
