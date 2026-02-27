package anyllm

// layerTensorName builds a tensor name like "blk.{layer}.{suffix}"
func layerTensorName(layer int, suffix string) string {
	nm := "blk."
	nm += intToString(layer)
	nm += "."
	nm += suffix
	return nm
}

// ComputeCacheElements computes the total elements needed to cache
// all non-expert tensors (norms, attention, embedding, output, routing gates)
func ComputeCacheElements(file GGUFFile, cfg ModelConfig) int {
	total := int64(0)

	// Embedding
	embIdx := FindTensorByName(file, "token_embd.weight")
	if embIdx >= 0 {
		total = total + TensorElementCount(file.Tensors[embIdx])
	}

	// Output
	outIdx := FindTensorByName(file, "output.weight")
	if outIdx >= 0 {
		total = total + TensorElementCount(file.Tensors[outIdx])
	}

	// Output norm
	normIdx := FindTensorByName(file, "output_norm.weight")
	if normIdx >= 0 {
		total = total + TensorElementCount(file.Tensors[normIdx])
	}

	// Per-layer tensors
	layer := 0
	for layer < cfg.BlockCount {
		// Attention norm
		anIdx := FindTensorByName(file, layerTensorName(layer, "attn_norm.weight"))
		if anIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[anIdx])
		}

		// FFN norm
		fnIdx := FindTensorByName(file, layerTensorName(layer, "ffn_norm.weight"))
		if fnIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[fnIdx])
		}

		// Attention Q
		qIdx := FindTensorByName(file, layerTensorName(layer, "attn_q.weight"))
		if qIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[qIdx])
		}

		// Attention K
		kIdx := FindTensorByName(file, layerTensorName(layer, "attn_k.weight"))
		if kIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[kIdx])
		}

		// Attention V
		vIdx := FindTensorByName(file, layerTensorName(layer, "attn_v.weight"))
		if vIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[vIdx])
		}

		// Attention output
		oIdx := FindTensorByName(file, layerTensorName(layer, "attn_output.weight"))
		if oIdx >= 0 {
			total = total + TensorElementCount(file.Tensors[oIdx])
		}

		// Routing gate (MoE)
		if cfg.ExpertCount > 0 {
			rIdx := FindTensorByName(file, layerTensorName(layer, "ffn_gate_inp.weight"))
			if rIdx >= 0 {
				total = total + TensorElementCount(file.Tensors[rIdx])
			}
		}

		layer = layer + 1
	}

	return int(total)
}

// NewTensorCache creates a tensor cache sized to hold all non-expert tensors
func NewTensorCache(file GGUFFile, cfg ModelConfig) TensorCache {
	tc := TensorCache{}
	numTensors := len(file.Tensors)
	attnElements := ComputeCacheElements(file, cfg)

	ffnSlotSize := 0
	if cfg.FFNLength > 0 {
		ffnSlotSize = cfg.FFNLength * cfg.EmbeddingLength
	}
	maxElements := attnElements + 3*ffnSlotSize

	tc.Entries = make([]float64, maxElements)
	tc.Offsets = make([]int, numTensors)
	tc.Counts = make([]int, numTensors)
	tc.UsedArr = make([]int, 1)
	tc.UsedArr[0] = 0

	ci := 0
	for ci < numTensors {
		tc.Offsets[ci] = -1
		tc.Counts[ci] = int(TensorElementCount(file.Tensors[ci]))
		ci = ci + 1
	}

	if ffnSlotSize > 0 {
		tc.FFNSlot1 = attnElements
		tc.FFNSlot2 = attnElements + ffnSlotSize
		tc.FFNSlot3 = attnElements + 2*ffnSlotSize
	} else {
		tc.FFNSlot1 = -1
		tc.FFNSlot2 = -1
		tc.FFNSlot3 = -1
	}

	return tc
}

// CacheOffset returns the offset of a tensor in the cache, or -1 if not cached
func CacheOffset(tc TensorCache, tensorIdx int) int {
	if tensorIdx < 0 || tensorIdx >= len(tc.Offsets) {
		return -1
	}
	return tc.Offsets[tensorIdx]
}

// ReadTensorCached reads a tensor, returning cached data if available
func ReadTensorCached(file GGUFFile, tensorIdx int, tc TensorCache) []float64 {
	noData := []float64{}
	if tensorIdx < 0 || tensorIdx >= len(tc.Offsets) {
		return noData
	}

	off := tc.Offsets[tensorIdx]
	count := tc.Counts[tensorIdx]

	// Return from cache if already loaded
	if off >= 0 {
		cached := make([]float64, count)
		ri := 0
		for ri < count {
			cached[ri] = tc.Entries[off+ri]
			ri = ri + 1
		}
		return cached
	}

	// Read from disk
	freshData := ReadTensor(file, tensorIdx)
	if len(freshData) == 0 {
		return freshData
	}

	// Store in cache if there is room
	used := tc.UsedArr[0]
	if used+count <= len(tc.Entries) {
		wi := 0
		for wi < count {
			tc.Entries[used+wi] = freshData[wi]
			wi = wi + 1
		}
		tc.Offsets[tensorIdx] = used
		tc.UsedArr[0] = used + count
	}

	return freshData
}

// PrewarmCache loads all non-expert tensors into cache before generation
func PrewarmCache(file GGUFFile, cfg ModelConfig, tc TensorCache) {
	// Cache embedding
	embIdx := FindTensorByName(file, "token_embd.weight")
	if embIdx >= 0 {
		ReadTensorCached(file, embIdx, tc)
	}

	// Cache output norm
	normIdx := FindTensorByName(file, "output_norm.weight")
	if normIdx >= 0 {
		ReadTensorCached(file, normIdx, tc)
	}

	// Cache output weights
	outIdx := FindTensorByName(file, "output.weight")
	if outIdx >= 0 {
		ReadTensorCached(file, outIdx, tc)
	}

	// Cache per-layer tensors
	layer := 0
	for layer < cfg.BlockCount {
		// Attention norm
		anIdx := FindTensorByName(file, layerTensorName(layer, "attn_norm.weight"))
		if anIdx >= 0 {
			ReadTensorCached(file, anIdx, tc)
		}

		// FFN norm
		fnIdx := FindTensorByName(file, layerTensorName(layer, "ffn_norm.weight"))
		if fnIdx >= 0 {
			ReadTensorCached(file, fnIdx, tc)
		}

		// Attention Q
		qIdx := FindTensorByName(file, layerTensorName(layer, "attn_q.weight"))
		if qIdx >= 0 {
			ReadTensorCached(file, qIdx, tc)
		}

		// Attention K
		kIdx := FindTensorByName(file, layerTensorName(layer, "attn_k.weight"))
		if kIdx >= 0 {
			ReadTensorCached(file, kIdx, tc)
		}

		// Attention V
		vIdx := FindTensorByName(file, layerTensorName(layer, "attn_v.weight"))
		if vIdx >= 0 {
			ReadTensorCached(file, vIdx, tc)
		}

		// Attention output
		oIdx := FindTensorByName(file, layerTensorName(layer, "attn_output.weight"))
		if oIdx >= 0 {
			ReadTensorCached(file, oIdx, tc)
		}

		// Routing gate (MoE)
		if cfg.ExpertCount > 0 {
			rIdx := FindTensorByName(file, layerTensorName(layer, "ffn_gate_inp.weight"))
			if rIdx >= 0 {
				ReadTensorCached(file, rIdx, tc)
			}
		}

		layer = layer + 1
	}
}
