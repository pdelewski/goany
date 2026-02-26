package anyllm

// KVCache holds key-value cache for transformer layers
type KVCache struct {
	K         []float64
	V         []float64
	MaxSeqLen int
	NumLayers int
	KVDim     int
}

// NewKVCache creates a new KV cache
func NewKVCache(maxSeqLen int, numLayers int, kvDim int) KVCache {
	cache := KVCache{}
	cache.MaxSeqLen = maxSeqLen
	cache.NumLayers = numLayers
	cache.KVDim = kvDim
	totalSize := maxSeqLen * numLayers * kvDim
	cache.K = make([]float64, totalSize)
	cache.V = make([]float64, totalSize)
	return cache
}

// kvOffset computes the offset into the KV cache
func kvOffset(cache KVCache, layer int, pos int) int {
	return layer*cache.MaxSeqLen*cache.KVDim + pos*cache.KVDim
}

// SetKV stores key and value vectors for a given layer and position
func SetKV(cache KVCache, layer int, pos int, key []float64, val []float64) {
	off := kvOffset(cache, layer, pos)
	i := 0
	for i < cache.KVDim {
		cache.K[off+i] = key[i]
		cache.V[off+i] = val[i]
		i = i + 1
	}
}

// GetK returns the key vector for a given layer and position
func GetK(cache KVCache, layer int, pos int) []float64 {
	off := kvOffset(cache, layer, pos)
	result := make([]float64, cache.KVDim)
	i := 0
	for i < cache.KVDim {
		result[i] = cache.K[off+i]
		i = i + 1
	}
	return result
}

// GetV returns the value vector for a given layer and position
func GetV(cache KVCache, layer int, pos int) []float64 {
	off := kvOffset(cache, layer, pos)
	result := make([]float64, cache.KVDim)
	i := 0
	for i < cache.KVDim {
		result[i] = cache.V[off+i]
		i = i + 1
	}
	return result
}
