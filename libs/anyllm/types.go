package anyllm

// GGUF magic number
const GGUFMagic int = 0x46554747

// GGUF metadata value types
const GGUFTypeUint8 int = 0
const GGUFTypeInt8 int = 1
const GGUFTypeUint16 int = 2
const GGUFTypeInt16 int = 3
const GGUFTypeUint32 int = 4
const GGUFTypeInt32 int = 5
const GGUFTypeFloat32 int = 6
const GGUFTypeBool int = 7
const GGUFTypeString int = 8
const GGUFTypeArray int = 9
const GGUFTypeUint64 int = 10
const GGUFTypeInt64 int = 11
const GGUFTypeFloat64 int = 12

// GGML tensor data types
const GGMLTypeF32 int = 0
const GGMLTypeF16 int = 1
const GGMLTypeQ4_0 int = 2
const GGMLTypeQ4_1 int = 3
const GGMLTypeQ5_0 int = 6
const GGMLTypeQ5_1 int = 7
const GGMLTypeQ8_0 int = 8
const GGMLTypeQ8_1 int = 9
const GGMLTypeQ2K int = 10
const GGMLTypeQ3K int = 11
const GGMLTypeQ4K int = 12
const GGMLTypeQ5K int = 13
const GGMLTypeQ6K int = 14
const GGMLTypeQ8K int = 15
const GGMLTypeBF16 int = 30

// GGUFHeader holds the file header information
type GGUFHeader struct {
	Magic         int
	Version       int
	TensorCount   int64
	MetadataCount int64
}

// GGUFMetadata holds a single metadata key-value pair
type GGUFMetadata struct {
	Key         string
	ValueType   int
	IntVal      int64
	FloatVal    float64
	BoolVal     bool
	StringVal   string
	ArrayLen    int64
	ArrayType   int
	ValueOffset int64
}

// ModelConfig holds extracted model configuration
type ModelConfig struct {
	Architecture    string
	EmbeddingLength int
	BlockCount      int
	HeadCount       int
	HeadCountKV     int
	VocabSize       int
	ContextLength   int
	FFNLength       int
	RopeFreqBase    float64
	RMSNormEps      float64
	ExpertCount     int
	ExpertUsedCount int
	Error           string
}

// GGUFTensorInfo holds information about a single tensor
type GGUFTensorInfo struct {
	Name   string
	NDims  int
	Dims   []int64
	DType  int
	Offset int64
}

// GGUFFile holds the complete parsed GGUF file
type GGUFFile struct {
	Header         GGUFHeader
	Metadata       []GGUFMetadata
	Tensors        []GGUFTensorInfo
	Error          string
	Handle         int
	DataBaseOffset int64
}

// TensorCache holds cached dequantized tensor data in a flat array
type TensorCache struct {
	Entries  []float64
	Offsets  []int
	Counts   []int
	UsedArr  []int
	FFNSlot1 int
	FFNSlot2 int
	FFNSlot3 int
}

// GenerateParams holds parameters for text generation
type GenerateParams struct {
	Temperature float64
	TopP        float64
	ChatFormat  int
	Seed        int64
}

// ReadState tracks the state of binary reading
type ReadState struct {
	Handle int
	Pos    int64
	Err    string
}
