package anyllm

// GGUFTypeName returns the name of a GGUF metadata value type
func GGUFTypeName(typeVal int) string {
	if typeVal == GGUFTypeUint8 {
		return "uint8"
	} else if typeVal == GGUFTypeInt8 {
		return "int8"
	} else if typeVal == GGUFTypeUint16 {
		return "uint16"
	} else if typeVal == GGUFTypeInt16 {
		return "int16"
	} else if typeVal == GGUFTypeUint32 {
		return "uint32"
	} else if typeVal == GGUFTypeInt32 {
		return "int32"
	} else if typeVal == GGUFTypeFloat32 {
		return "float32"
	} else if typeVal == GGUFTypeBool {
		return "bool"
	} else if typeVal == GGUFTypeString {
		return "string"
	} else if typeVal == GGUFTypeArray {
		return "array"
	} else if typeVal == GGUFTypeUint64 {
		return "uint64"
	} else if typeVal == GGUFTypeInt64 {
		return "int64"
	} else if typeVal == GGUFTypeFloat64 {
		return "float64"
	}
	return "unknown"
}

// GGMLTypeName returns the name of a GGML tensor data type
func GGMLTypeName(typeVal int) string {
	if typeVal == GGMLTypeF32 {
		return "F32"
	} else if typeVal == GGMLTypeF16 {
		return "F16"
	} else if typeVal == GGMLTypeQ4_0 {
		return "Q4_0"
	} else if typeVal == GGMLTypeQ4_1 {
		return "Q4_1"
	} else if typeVal == GGMLTypeQ5_0 {
		return "Q5_0"
	} else if typeVal == GGMLTypeQ5_1 {
		return "Q5_1"
	} else if typeVal == GGMLTypeQ8_0 {
		return "Q8_0"
	} else if typeVal == GGMLTypeQ8_1 {
		return "Q8_1"
	} else if typeVal == GGMLTypeQ2K {
		return "Q2_K"
	} else if typeVal == GGMLTypeQ3K {
		return "Q3_K"
	} else if typeVal == GGMLTypeQ4K {
		return "Q4_K"
	} else if typeVal == GGMLTypeQ5K {
		return "Q5_K"
	} else if typeVal == GGMLTypeQ6K {
		return "Q6_K"
	} else if typeVal == GGMLTypeQ8K {
		return "Q8_K"
	} else if typeVal == GGMLTypeBF16 {
		return "BF16"
	}
	return "unknown"
}

func formatMetadataValue(md GGUFMetadata) string {
	vt := md.ValueType
	if vt == GGUFTypeUint8 || vt == GGUFTypeInt8 || vt == GGUFTypeUint16 || vt == GGUFTypeInt16 || vt == GGUFTypeUint32 || vt == GGUFTypeInt32 || vt == GGUFTypeUint64 || vt == GGUFTypeInt64 {
		return int64ToString(md.IntVal)
	} else if vt == GGUFTypeFloat32 || vt == GGUFTypeFloat64 {
		return float64ToString(md.FloatVal)
	} else if vt == GGUFTypeBool {
		if md.BoolVal {
			return "true"
		}
		return "false"
	} else if vt == GGUFTypeString {
		sv := md.StringVal
		if len(sv) > 80 {
			sv2 := ""
			j := 0
			for j < 80 {
				ch := int(sv[j])
				sv2 += charToString(ch)
				j = j + 1
			}
			sv2 += "..."
			return sv2
		}
		return sv
	} else if vt == GGUFTypeArray {
		arrInfo := "["
		arrInfo += GGUFTypeName(md.ArrayType)
		arrInfo += " x "
		arrInfo += int64ToString(md.ArrayLen)
		arrInfo += "]"
		return arrInfo
	}
	return "?"
}

// PrintSummary returns a human-readable summary of a parsed GGUF file
func PrintSummary(file GGUFFile) string {
	if file.Error != "" {
		errMsg := "Error: "
		errMsg += file.Error
		errMsg += "\n"
		return errMsg
	}

	res := "=== GGUF File Summary ===\n"
	res += "Magic: 0x"
	res += intToHex(file.Header.Magic)
	res += "\n"
	res += "Version: "
	res += intToString(file.Header.Version)
	res += "\n"
	res += "Tensor count: "
	res += int64ToString(file.Header.TensorCount)
	res += "\n"
	res += "Metadata count: "
	res += int64ToString(file.Header.MetadataCount)
	res += "\n"

	res += "\n--- Metadata ---\n"
	i := 0
	for i < len(file.Metadata) {
		md := file.Metadata[i]
		res += "  "
		res += md.Key
		res += " ("
		res += GGUFTypeName(md.ValueType)
		res += "): "
		res += formatMetadataValue(md)
		res += "\n"
		i = i + 1
	}

	res += "\n--- Tensors ---\n"
	j := 0
	for j < len(file.Tensors) {
		t := file.Tensors[j]
		res += "  "
		res += t.Name
		res += " ["
		res += GGMLTypeName(t.DType)
		res += "] shape=("
		k := 0
		for k < t.NDims {
			if k > 0 {
				res += ", "
			}
			res += int64ToString(t.Dims[k])
			k = k + 1
		}
		res += ") offset="
		res += int64ToString(t.Offset)
		res += "\n"
		j = j + 1
	}

	return res
}

func intToHex(n int) string {
	if n == 0 {
		return "0"
	}
	hexChars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "A", "B", "C", "D", "E", "F"}
	result := ""
	val := n
	for val > 0 {
		digit := val % 16
		result = hexChars[digit] + result
		val = val / 16
	}
	return result
}
