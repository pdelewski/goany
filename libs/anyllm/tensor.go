package anyllm

import "runtime/fs"

// TensorElementCount returns the total number of elements in a tensor
func TensorElementCount(tensor GGUFTensorInfo) int64 {
	count := int64(1)
	i := 0
	for i < tensor.NDims {
		count = count * tensor.Dims[i]
		i = i + 1
	}
	return count
}

// K-quant constants
const QK_K int = 256
const Q2K_BlockSize int = 84
const Q3K_BlockSize int = 110
const Q6K_BlockSize int = 210

// TensorByteSize returns the byte size of a tensor's data
func TensorByteSize(tensor GGUFTensorInfo) int64 {
	elements := TensorElementCount(tensor)
	if tensor.DType == GGMLTypeF32 {
		return elements * 4
	} else if tensor.DType == GGMLTypeF16 {
		return elements * 2
	} else if tensor.DType == GGMLTypeQ8_0 {
		numBlocks := elements / 32
		return numBlocks * 34
	} else if tensor.DType == GGMLTypeQ4_0 {
		numBlocks := elements / 32
		return numBlocks * 20
	} else if tensor.DType == GGMLTypeQ4_1 {
		numBlocks := elements / 32
		return numBlocks * 24
	} else if tensor.DType == GGMLTypeQ2K {
		numBlocks := elements / int64(QK_K)
		return numBlocks * int64(Q2K_BlockSize)
	} else if tensor.DType == GGMLTypeQ3K {
		numBlocks := elements / int64(QK_K)
		return numBlocks * int64(Q3K_BlockSize)
	} else if tensor.DType == GGMLTypeQ6K {
		numBlocks := elements / int64(QK_K)
		return numBlocks * int64(Q6K_BlockSize)
	}
	return elements * 4
}

// ReadTensorRaw reads raw bytes for a tensor from the file
func ReadTensorRaw(file GGUFFile, tensorIdx int) []byte {
	if tensorIdx < 0 || tensorIdx >= len(file.Tensors) {
		empty := []byte{}
		return empty
	}
	tensor := file.Tensors[tensorIdx]
	offset := file.DataBaseOffset + tensor.Offset
	byteSize := TensorByteSize(tensor)
	rawData, bytesRead, err := fs.ReadAt(file.Handle, offset, int(byteSize))
	if err != "" {
		empty := []byte{}
		return empty
	}
	result := []byte{}
	ri := 0
	for ri < bytesRead {
		result = append(result, rawData[ri])
		ri = ri + 1
	}
	return result
}

// ReadTensorF32 reads an F32 tensor and returns float64 values
func ReadTensorF32(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	result := make([]float64, elements)
	i := 0
	for i < elements {
		off := i * 4
		result[i] = decodeF32Bytes(data, off)
		i = i + 1
	}
	return result
}

// ReadTensorF16 reads an F16 tensor and returns float64 values
func ReadTensorF16(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	result := make([]float64, elements)
	i := 0
	for i < elements {
		off := i * 2
		result[i] = decodeF16(data, off)
		i = i + 1
	}
	return result
}

// ReadTensorQ8_0 reads a Q8_0 tensor and returns dequantized float64 values
func ReadTensorQ8_0(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	numBlocks := elements / 32
	result := make([]float64, elements)
	bi := 0
	for bi < numBlocks {
		blockOffset := bi * 34
		blockVals := dequantQ8_0Block(data, blockOffset)
		j := 0
		for j < 32 {
			result[bi*32+j] = blockVals[j]
			j = j + 1
		}
		bi = bi + 1
	}
	return result
}

// ReadTensorQ2K reads a Q2_K tensor and returns dequantized float64 values
func ReadTensorQ2K(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	numBlocks := elements / QK_K
	result := make([]float64, elements)
	bi := 0
	for bi < numBlocks {
		dequantQ2K(data, bi*Q2K_BlockSize, result, bi*QK_K)
		bi = bi + 1
	}
	return result
}

// ReadTensorQ3K reads a Q3_K tensor and returns dequantized float64 values
func ReadTensorQ3K(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	numBlocks := elements / QK_K
	result := make([]float64, elements)
	bi := 0
	for bi < numBlocks {
		dequantQ3K(data, bi*Q3K_BlockSize, result, bi*QK_K)
		bi = bi + 1
	}
	return result
}

// ReadTensorQ6K reads a Q6_K tensor and returns dequantized float64 values
func ReadTensorQ6K(file GGUFFile, tensorIdx int) []float64 {
	tensor := file.Tensors[tensorIdx]
	elements := int(TensorElementCount(tensor))
	data := ReadTensorRaw(file, tensorIdx)
	if len(data) == 0 {
		empty := []float64{}
		return empty
	}
	numBlocks := elements / QK_K
	result := make([]float64, elements)
	bi := 0
	for bi < numBlocks {
		dequantQ6K(data, bi*Q6K_BlockSize, result, bi*QK_K)
		bi = bi + 1
	}
	return result
}

// ReadTensor reads any supported tensor type and returns float64 values
func ReadTensor(file GGUFFile, tensorIdx int) []float64 {
	noData := []float64{}
	if tensorIdx < 0 || tensorIdx >= len(file.Tensors) {
		return noData
	}
	tensor := file.Tensors[tensorIdx]
	if tensor.DType == GGMLTypeF32 {
		return ReadTensorF32(file, tensorIdx)
	} else if tensor.DType == GGMLTypeF16 {
		return ReadTensorF16(file, tensorIdx)
	} else if tensor.DType == GGMLTypeQ8_0 {
		return ReadTensorQ8_0(file, tensorIdx)
	} else if tensor.DType == GGMLTypeQ2K {
		return ReadTensorQ2K(file, tensorIdx)
	} else if tensor.DType == GGMLTypeQ3K {
		return ReadTensorQ3K(file, tensorIdx)
	} else if tensor.DType == GGMLTypeQ6K {
		return ReadTensorQ6K(file, tensorIdx)
	}
	return noData
}
