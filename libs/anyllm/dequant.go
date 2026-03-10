package anyllm

// decodeF32Bytes decodes 4 little-endian bytes as float32 -> float64
func decodeF32Bytes(data []byte, offset int) float64 {
	b0 := int(data[offset])
	b1 := int(data[offset+1])
	b2 := int(data[offset+2])
	b3 := int(data[offset+3])
	bits := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
	sign := (bits >> 31) & 1
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF
	if exp == 0 && frac == 0 {
		return 0.0
	}
	e := float64(exp - 127)
	m := 1.0 + float64(frac)/8388608.0
	val := m * pow2(e)
	if sign == 1 {
		val = -val
	}
	return val
}

// dequantQ8_0Block dequantizes a single Q8_0 block (34 bytes -> 32 float64 weights)
// Format: 2 bytes f16 scale + 32 bytes int8 values
func dequantQ8_0Block(data []byte, offset int) []float64 {
	scale := decodeF16(data, offset)
	result := make([]float64, 32)
	i := 0
	for i < 32 {
		raw := int(data[offset+2+i])
		val := raw
		if val > 127 {
			val = val - 256
		}
		result[i] = scale * float64(val)
		i = i + 1
	}
	return result
}

// decodeF16 decodes a 2-byte IEEE 754 half-precision float to float64
func decodeF16(data []byte, offset int) float64 {
	b0 := int(data[offset])
	b1 := int(data[offset+1])
	bits := b0 | (b1 << 8)
	sign := (bits >> 15) & 1
	exp := (bits >> 10) & 0x1F
	frac := bits & 0x3FF
	if exp == 0 && frac == 0 {
		if sign == 1 {
			return -0.0
		}
		return 0.0
	}
	if exp == 0 {
		val := float64(frac) / 1024.0 * pow2(-14.0)
		if sign == 1 {
			val = -val
		}
		return val
	}
	if exp == 31 {
		return 0.0
	}
	e := float64(exp - 15)
	m := 1.0 + float64(frac)/1024.0
	result := m * pow2(e)
	if sign == 1 {
		result = -result
	}
	return result
}

// dequantQ2K dequantizes a Q2_K super-block (256 elements, 84 bytes)
// Layout: scales[16] + qs[64] + d(f16,2B) + dmin(f16,2B)
// Each byte in qs holds 4 x 2-bit values (at shifts 0,2,4,6)
func dequantQ2K(data []byte, blockOffset int, output []float64, outOffset int) []float64 {
	dVal := decodeF16(data, blockOffset+80)
	dminVal := decodeF16(data, blockOffset+82)

	scBase := blockOffset
	qBase := blockOffset + 16

	si := 0
	outPos := outOffset
	// Two 128-element chunks
	chunk := 0
	for chunk < 2 {
		qp := qBase + chunk*32
		shift := 0
		for shift < 8 {
			sc := int(data[scBase+si])
			si = si + 1
			dl := dVal * float64(sc&0xF)
			ml := dminVal * float64(sc>>4)
			l := 0
			for l < 16 {
				qByte := int(data[qp+l])
				qVal := (qByte >> shift) & 3
				output[outPos] = dl*float64(qVal) - ml
				outPos = outPos + 1
				l = l + 1
			}

			sc2 := int(data[scBase+si])
			si = si + 1
			dl2 := dVal * float64(sc2&0xF)
			ml2 := dminVal * float64(sc2>>4)
			l2 := 0
			for l2 < 16 {
				qByte := int(data[qp+16+l2])
				qVal := (qByte >> shift) & 3
				output[outPos] = dl2*float64(qVal) - ml2
				outPos = outPos + 1
				l2 = l2 + 1
			}

			shift = shift + 2
		}
		chunk = chunk + 1
	}
	return output
}

// dequantQ3K dequantizes a Q3_K super-block (256 elements, 110 bytes)
// Layout: hmask[32] + qs[64] + scales[12] + d(f16,2B)
func dequantQ3K(data []byte, blockOffset int, output []float64, outOffset int) []float64 {
	hmaskBase := blockOffset
	qsStart := blockOffset + 32
	scalesBase := blockOffset + 96
	dAll := decodeF16(data, blockOffset+108)

	// Decode 6-bit scales from 12 bytes into 16 values
	a0 := int(data[scalesBase]) | (int(data[scalesBase+1]) << 8) | (int(data[scalesBase+2]) << 16) | (int(data[scalesBase+3]) << 24)
	a1 := int(data[scalesBase+4]) | (int(data[scalesBase+5]) << 8) | (int(data[scalesBase+6]) << 16) | (int(data[scalesBase+7]) << 24)
	a2 := int(data[scalesBase+8]) | (int(data[scalesBase+9]) << 8) | (int(data[scalesBase+10]) << 16) | (int(data[scalesBase+11]) << 24)

	km2 := 0x0f0f0f0f
	km1 := 0x03030303

	tmp := a2
	s2 := ((a0 >> 4) & km2) | (((tmp >> 4) & km1) << 4)
	s3 := ((a1 >> 4) & km2) | (((tmp >> 6) & km1) << 4)
	s0 := (a0 & km2) | (((tmp >> 0) & km1) << 4)
	s1 := (a1 & km2) | (((tmp >> 2) & km1) << 4)

	scales := make([]int, 16)
	scales[0] = (s0 & 0xFF) - 32
	scales[1] = ((s0 >> 8) & 0xFF) - 32
	scales[2] = ((s0 >> 16) & 0xFF) - 32
	scales[3] = ((s0 >> 24) & 0xFF) - 32
	scales[4] = (s1 & 0xFF) - 32
	scales[5] = ((s1 >> 8) & 0xFF) - 32
	scales[6] = ((s1 >> 16) & 0xFF) - 32
	scales[7] = ((s1 >> 24) & 0xFF) - 32
	scales[8] = (s2 & 0xFF) - 32
	scales[9] = ((s2 >> 8) & 0xFF) - 32
	scales[10] = ((s2 >> 16) & 0xFF) - 32
	scales[11] = ((s2 >> 24) & 0xFF) - 32
	scales[12] = (s3 & 0xFF) - 32
	scales[13] = ((s3 >> 8) & 0xFF) - 32
	scales[14] = ((s3 >> 16) & 0xFF) - 32
	scales[15] = ((s3 >> 24) & 0xFF) - 32

	outPos := outOffset
	mBit := 1
	scIdx := 0

	// Two 128-element chunks
	chunk := 0
	for chunk < 2 {
		qp := qsStart + chunk*32
		shift := 0
		for shift < 8 {
			dl := dAll * float64(scales[scIdx])
			scIdx = scIdx + 1
			l := 0
			for l < 16 {
				qByte := int(data[qp+l])
				hmByte := int(data[hmaskBase+l])
				q2bit := (qByte >> shift) & 3
				highBit := 0
				if (hmByte & mBit) == 0 {
					highBit = 4
				}
				output[outPos] = dl * float64(q2bit-highBit)
				outPos = outPos + 1
				l = l + 1
			}

			dl2 := dAll * float64(scales[scIdx])
			scIdx = scIdx + 1
			l2 := 0
			for l2 < 16 {
				qByte := int(data[qp+16+l2])
				hmByte := int(data[hmaskBase+16+l2])
				q2bit := (qByte >> shift) & 3
				highBit := 0
				if (hmByte & mBit) == 0 {
					highBit = 4
				}
				output[outPos] = dl2 * float64(q2bit-highBit)
				outPos = outPos + 1
				l2 = l2 + 1
			}

			shift = shift + 2
			mBit = mBit << 1
		}
		chunk = chunk + 1
	}
	return output
}

// dequantQ6K dequantizes a Q6_K super-block (256 elements, 210 bytes)
// Layout: ql[128] + qh[64] + scales[16] + d(f16,2B)
func dequantQ6K(data []byte, blockOffset int, output []float64, outOffset int) []float64 {
	qlBase := blockOffset
	qhBase := blockOffset + 128
	scBase := blockOffset + 192
	dVal := decodeF16(data, blockOffset+208)

	qlP := qlBase
	qhP := qhBase
	scP := scBase
	outPos := outOffset

	chunk := 0
	for chunk < 2 {
		l := 0
		for l < 32 {
			scOff := l / 16

			qlByte0 := int(data[qlP+l])
			qlByte32 := int(data[qlP+l+32])
			qhByte := int(data[qhP+l])

			q1 := (qlByte0 & 0xF) | (((qhByte >> 0) & 3) << 4)
			q1 = q1 - 32
			q2 := (qlByte32 & 0xF) | (((qhByte >> 2) & 3) << 4)
			q2 = q2 - 32
			q3 := (qlByte0 >> 4) | (((qhByte >> 4) & 3) << 4)
			q3 = q3 - 32
			q4 := (qlByte32 >> 4) | (((qhByte >> 6) & 3) << 4)
			q4 = q4 - 32

			sc0 := int(data[scP+scOff])
			sc2 := int(data[scP+scOff+2])
			sc4 := int(data[scP+scOff+4])
			sc6 := int(data[scP+scOff+6])
			if sc0 > 127 {
				sc0 = sc0 - 256
			}
			if sc2 > 127 {
				sc2 = sc2 - 256
			}
			if sc4 > 127 {
				sc4 = sc4 - 256
			}
			if sc6 > 127 {
				sc6 = sc6 - 256
			}

			output[outPos+l] = dVal * float64(sc0) * float64(q1)
			output[outPos+l+32] = dVal * float64(sc2) * float64(q2)
			output[outPos+l+64] = dVal * float64(sc4) * float64(q3)
			output[outPos+l+96] = dVal * float64(sc6) * float64(q4)

			l = l + 1
		}
		outPos = outPos + 128
		qlP = qlP + 64
		qhP = qhP + 32
		scP = scP + 8
		chunk = chunk + 1
	}
	return output
}
