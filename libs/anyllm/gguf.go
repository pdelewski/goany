package anyllm

import "runtime/fs"

func metadataValueSize(valueType int) int {
	if valueType == GGUFTypeUint8 {
		return 1
	} else if valueType == GGUFTypeInt8 {
		return 1
	} else if valueType == GGUFTypeUint16 {
		return 2
	} else if valueType == GGUFTypeInt16 {
		return 2
	} else if valueType == GGUFTypeUint32 {
		return 4
	} else if valueType == GGUFTypeInt32 {
		return 4
	} else if valueType == GGUFTypeFloat32 {
		return 4
	} else if valueType == GGUFTypeBool {
		return 1
	} else if valueType == GGUFTypeUint64 {
		return 8
	} else if valueType == GGUFTypeInt64 {
		return 8
	} else if valueType == GGUFTypeFloat64 {
		return 8
	}
	return 0
}

func skipMetadataValue(rs ReadState, valueType int) ReadState {
	if valueType == GGUFTypeString {
		rs2, _ := readGGUFString(rs)
		return rs2
	}
	sz := metadataValueSize(valueType)
	if sz > 0 {
		return skipBytes(rs, int64(sz))
	}
	return rs
}

func readMetadataValue(rs ReadState, valueType int) (ReadState, GGUFMetadata) {
	md := GGUFMetadata{}
	md.ValueType = valueType
	if valueType == GGUFTypeUint8 {
		rs2, val := readUint8(rs)
		md.IntVal = int64(val)
		return rs2, md
	} else if valueType == GGUFTypeInt8 {
		rs2, val := readUint8(rs)
		if val > 127 {
			val = val - 256
		}
		md.IntVal = int64(val)
		return rs2, md
	} else if valueType == GGUFTypeUint16 {
		rs2, data := readBytes(rs, 2)
		if rs2.Err != "" {
			return rs2, md
		}
		b0 := int(data[0])
		b1 := int(data[1])
		val := b0 | (b1 << 8)
		md.IntVal = int64(val)
		return rs2, md
	} else if valueType == GGUFTypeInt16 {
		rs2, data := readBytes(rs, 2)
		if rs2.Err != "" {
			return rs2, md
		}
		b0 := int(data[0])
		b1 := int(data[1])
		val := b0 | (b1 << 8)
		if val > 32767 {
			val = val - 65536
		}
		md.IntVal = int64(val)
		return rs2, md
	} else if valueType == GGUFTypeUint32 {
		rs2, val := readUint32LE(rs)
		md.IntVal = int64(val)
		return rs2, md
	} else if valueType == GGUFTypeInt32 {
		rs2, val := readUint32LE(rs)
		signedVal := int64(val)
		if signedVal > 2147483647 {
			signedVal = signedVal - 4294967296
		}
		md.IntVal = signedVal
		return rs2, md
	} else if valueType == GGUFTypeFloat32 {
		rs2, val := readFloat32LE(rs)
		md.FloatVal = val
		return rs2, md
	} else if valueType == GGUFTypeBool {
		rs2, val := readUint8(rs)
		if val != 0 {
			md.BoolVal = true
		} else {
			md.BoolVal = false
		}
		return rs2, md
	} else if valueType == GGUFTypeString {
		rs2, val := readGGUFString(rs)
		md.StringVal = val
		return rs2, md
	} else if valueType == GGUFTypeUint64 {
		rs2, val := readUint64LE(rs)
		md.IntVal = val
		return rs2, md
	} else if valueType == GGUFTypeInt64 {
		rs2, val := readUint64LE(rs)
		md.IntVal = val
		return rs2, md
	} else if valueType == GGUFTypeFloat64 {
		rs2, val := readFloat64LE(rs)
		md.FloatVal = val
		return rs2, md
	} else if valueType == GGUFTypeArray {
		rs2, arrType := readUint32LE(rs)
		if rs2.Err != "" {
			return rs2, md
		}
		rs3, arrLen := readUint64LE(rs2)
		if rs3.Err != "" {
			return rs3, md
		}
		md.ArrayType = arrType
		md.ArrayLen = arrLen
		rs4 := rs3
		i := int64(0)
		for i < arrLen {
			rs4 = skipMetadataValue(rs4, arrType)
			if rs4.Err != "" {
				return rs4, md
			}
			i = i + 1
		}
		return rs4, md
	}
	rs.Err = "unknown metadata type"
	return rs, md
}

// LoadGGUF loads and parses a GGUF file from the given path
func LoadGGUF(path string) GGUFFile {
	result := GGUFFile{}
	result.Metadata = []GGUFMetadata{}
	result.Tensors = []GGUFTensorInfo{}

	handle, err := fs.Open(path)
	if err != "" {
		result.Error = err
		return result
	}

	rs := newReadState(handle)

	// Read magic
	rs2, magic := readUint32LE(rs)
	if rs2.Err != "" {
		result.Error = rs2.Err
		fs.Close(handle)
		return result
	}
	if magic != GGUFMagic {
		result.Error = "invalid GGUF magic number"
		fs.Close(handle)
		return result
	}
	result.Header.Magic = magic

	// Read version
	rs3, version := readUint32LE(rs2)
	if rs3.Err != "" {
		result.Error = rs3.Err
		fs.Close(handle)
		return result
	}
	if version != 2 && version != 3 {
		result.Error = "unsupported GGUF version"
		fs.Close(handle)
		return result
	}
	result.Header.Version = version

	// Read tensor count
	rs4, tensorCount := readUint64LE(rs3)
	if rs4.Err != "" {
		result.Error = rs4.Err
		fs.Close(handle)
		return result
	}
	result.Header.TensorCount = tensorCount

	// Read metadata count
	rs5, metadataCount := readUint64LE(rs4)
	if rs5.Err != "" {
		result.Error = rs5.Err
		fs.Close(handle)
		return result
	}
	result.Header.MetadataCount = metadataCount

	// Read metadata entries
	rsM := rs5
	mi := int64(0)
	for mi < metadataCount {
		rsM2, key := readGGUFString(rsM)
		if rsM2.Err != "" {
			result.Error = rsM2.Err
			fs.Close(handle)
			return result
		}
		rsM3, valueType := readUint32LE(rsM2)
		if rsM3.Err != "" {
			result.Error = rsM3.Err
			fs.Close(handle)
			return result
		}
		valueOffset := rsM3.Pos
		rsM4, md := readMetadataValue(rsM3, valueType)
		md.ValueOffset = valueOffset
		if rsM4.Err != "" {
			result.Error = rsM4.Err
			fs.Close(handle)
			return result
		}
		md.Key = key
		result.Metadata = append(result.Metadata, md)
		rsM = rsM4
		mi = mi + 1
	}

	// Read tensor info entries
	rsT := rsM
	ti := int64(0)
	for ti < tensorCount {
		rsT2, name := readGGUFString(rsT)
		if rsT2.Err != "" {
			result.Error = rsT2.Err
			fs.Close(handle)
			return result
		}
		rsT3, nDims := readUint32LE(rsT2)
		if rsT3.Err != "" {
			result.Error = rsT3.Err
			fs.Close(handle)
			return result
		}
		dims := []int64{}
		rsD := rsT3
		di := 0
		for di < nDims {
			rsD2, dim := readUint64LE(rsD)
			if rsD2.Err != "" {
				result.Error = rsD2.Err
				fs.Close(handle)
				return result
			}
			dims = append(dims, dim)
			rsD = rsD2
			di = di + 1
		}
		rsT4, dtype := readUint32LE(rsD)
		if rsT4.Err != "" {
			result.Error = rsT4.Err
			fs.Close(handle)
			return result
		}
		rsT5, offset := readUint64LE(rsT4)
		if rsT5.Err != "" {
			result.Error = rsT5.Err
			fs.Close(handle)
			return result
		}
		tensor := GGUFTensorInfo{}
		tensor.Name = name
		tensor.NDims = nDims
		tensor.Dims = dims
		tensor.DType = dtype
		tensor.Offset = offset
		result.Tensors = append(result.Tensors, tensor)
		rsT = rsT5
		ti = ti + 1
	}

	// Compute aligned data base offset (align to 32 bytes)
	endPos := rsT.Pos
	result.DataBaseOffset = ((endPos + 31) / 32) * 32

	fs.Close(handle)
	return result
}

// OpenGGUF parses a GGUF file and keeps the handle open for ReadAt access
func OpenGGUF(path string) GGUFFile {
	result := GGUFFile{}
	result.Metadata = []GGUFMetadata{}
	result.Tensors = []GGUFTensorInfo{}

	handle, err := fs.Open(path)
	if err != "" {
		result.Error = err
		return result
	}

	rs := newReadState(handle)

	// Read magic
	rs2, magic := readUint32LE(rs)
	if rs2.Err != "" {
		result.Error = rs2.Err
		fs.Close(handle)
		return result
	}
	if magic != GGUFMagic {
		result.Error = "invalid GGUF magic number"
		fs.Close(handle)
		return result
	}
	result.Header.Magic = magic

	// Read version
	rs3, version := readUint32LE(rs2)
	if rs3.Err != "" {
		result.Error = rs3.Err
		fs.Close(handle)
		return result
	}
	if version != 2 && version != 3 {
		result.Error = "unsupported GGUF version"
		fs.Close(handle)
		return result
	}
	result.Header.Version = version

	// Read tensor count
	rs4, tensorCount := readUint64LE(rs3)
	if rs4.Err != "" {
		result.Error = rs4.Err
		fs.Close(handle)
		return result
	}
	result.Header.TensorCount = tensorCount

	// Read metadata count
	rs5, metadataCount := readUint64LE(rs4)
	if rs5.Err != "" {
		result.Error = rs5.Err
		fs.Close(handle)
		return result
	}
	result.Header.MetadataCount = metadataCount

	// Read metadata entries
	rsM := rs5
	mi := int64(0)
	for mi < metadataCount {
		rsM2, key := readGGUFString(rsM)
		if rsM2.Err != "" {
			result.Error = rsM2.Err
			fs.Close(handle)
			return result
		}
		rsM3, valueType := readUint32LE(rsM2)
		if rsM3.Err != "" {
			result.Error = rsM3.Err
			fs.Close(handle)
			return result
		}
		valueOffset := rsM3.Pos
		rsM4, md := readMetadataValue(rsM3, valueType)
		md.ValueOffset = valueOffset
		if rsM4.Err != "" {
			result.Error = rsM4.Err
			fs.Close(handle)
			return result
		}
		md.Key = key
		result.Metadata = append(result.Metadata, md)
		rsM = rsM4
		mi = mi + 1
	}

	// Read tensor info entries
	rsT := rsM
	ti := int64(0)
	for ti < tensorCount {
		rsT2, name := readGGUFString(rsT)
		if rsT2.Err != "" {
			result.Error = rsT2.Err
			fs.Close(handle)
			return result
		}
		rsT3, nDims := readUint32LE(rsT2)
		if rsT3.Err != "" {
			result.Error = rsT3.Err
			fs.Close(handle)
			return result
		}
		dims := []int64{}
		rsD := rsT3
		di := 0
		for di < nDims {
			rsD2, dim := readUint64LE(rsD)
			if rsD2.Err != "" {
				result.Error = rsD2.Err
				fs.Close(handle)
				return result
			}
			dims = append(dims, dim)
			rsD = rsD2
			di = di + 1
		}
		rsT4, dtype := readUint32LE(rsD)
		if rsT4.Err != "" {
			result.Error = rsT4.Err
			fs.Close(handle)
			return result
		}
		rsT5, offset := readUint64LE(rsT4)
		if rsT5.Err != "" {
			result.Error = rsT5.Err
			fs.Close(handle)
			return result
		}
		tensor := GGUFTensorInfo{}
		tensor.Name = name
		tensor.NDims = nDims
		tensor.Dims = dims
		tensor.DType = dtype
		tensor.Offset = offset
		result.Tensors = append(result.Tensors, tensor)
		rsT = rsT5
		ti = ti + 1
	}

	// Compute aligned data base offset (align to 32 bytes)
	endPos := rsT.Pos
	result.DataBaseOffset = ((endPos + 31) / 32) * 32

	// Keep handle open for ReadAt access
	result.Handle = handle
	return result
}

// CloseGGUF closes the file handle of an opened GGUF file
func CloseGGUF(file GGUFFile) {
	if file.Handle > 0 {
		fs.Close(file.Handle)
	}
}

// FindMetadata finds a metadata entry by key and returns its index, or -1 if not found
func FindMetadata(file GGUFFile, key string) int {
	i := 0
	for i < len(file.Metadata) {
		if file.Metadata[i].Key == key {
			return i
		}
		i = i + 1
	}
	return -1
}

// GetMetadataInt returns the integer value of a metadata entry, or defaultVal if not found
func GetMetadataInt(file GGUFFile, key string, defaultVal int64) int64 {
	idx := FindMetadata(file, key)
	if idx < 0 {
		return defaultVal
	}
	return file.Metadata[idx].IntVal
}

// GetMetadataFloat returns the float value of a metadata entry, or defaultVal if not found
func GetMetadataFloat(file GGUFFile, key string, defaultVal float64) float64 {
	idx := FindMetadata(file, key)
	if idx < 0 {
		return defaultVal
	}
	return file.Metadata[idx].FloatVal
}

// GetMetadataString returns the string value of a metadata entry, or defaultVal if not found
func GetMetadataString(file GGUFFile, key string, defaultVal string) string {
	idx := FindMetadata(file, key)
	if idx < 0 {
		return defaultVal
	}
	return file.Metadata[idx].StringVal
}

// ReadMetadataStringArray reads a string array metadata value by seeking to its ValueOffset
func ReadMetadataStringArray(file GGUFFile, key string) []string {
	emptyStrs := []string{}
	idx := FindMetadata(file, key)
	if idx < 0 {
		return emptyStrs
	}
	md := file.Metadata[idx]
	if md.ValueType != GGUFTypeArray || md.ArrayType != GGUFTypeString {
		return emptyStrs
	}
	// Seek to the value offset (past the array type and length)
	rs := ReadState{}
	rs.Handle = file.Handle
	rs.Pos = md.ValueOffset
	rs.Err = ""
	// Skip array type (4 bytes) and array length (8 bytes)
	rs = skipBytes(rs, 12)
	if rs.Err != "" {
		return emptyStrs
	}
	result := []string{}
	ai := int64(0)
	for ai < md.ArrayLen {
		rs2, strVal := readGGUFString(rs)
		if rs2.Err != "" {
			return result
		}
		result = append(result, strVal)
		rs = rs2
		ai = ai + 1
	}
	return result
}

// ReadMetadataFloatArray reads a float array metadata value by seeking to its ValueOffset
func ReadMetadataFloatArray(file GGUFFile, key string) []float64 {
	emptyFloats := []float64{}
	idx := FindMetadata(file, key)
	if idx < 0 {
		return emptyFloats
	}
	md := file.Metadata[idx]
	if md.ValueType != GGUFTypeArray {
		return emptyFloats
	}
	// Seek to the value offset (past the array type and length)
	rs := ReadState{}
	rs.Handle = file.Handle
	rs.Pos = md.ValueOffset
	rs.Err = ""
	// Skip array type (4 bytes) and array length (8 bytes)
	rs = skipBytes(rs, 12)
	if rs.Err != "" {
		return emptyFloats
	}
	result := []float64{}
	ai := int64(0)
	for ai < md.ArrayLen {
		if md.ArrayType == GGUFTypeFloat32 {
			rs2, fval := readFloat32LE(rs)
			if rs2.Err != "" {
				return result
			}
			result = append(result, fval)
			rs = rs2
		} else if md.ArrayType == GGUFTypeFloat64 {
			rs2, fval := readFloat64LE(rs)
			if rs2.Err != "" {
				return result
			}
			result = append(result, fval)
			rs = rs2
		} else {
			return result
		}
		ai = ai + 1
	}
	return result
}
