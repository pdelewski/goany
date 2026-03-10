package anyllm

// ApplyRoPEVec applies Rotary Position Embedding to a single vector in-place
func ApplyRoPEVec(vec []float64, headDim int, pos int, freqBase float64) []float64 {
	halfDim := headDim / 2
	i := 0
	for i < halfDim {
		freq := 1.0 / Pow(freqBase, float64(2*i)/float64(headDim))
		angle := float64(pos) * freq
		cosVal := Cos(angle)
		sinVal := Sin(angle)

		v0 := vec[2*i]
		v1 := vec[2*i+1]
		vec[2*i] = v0*cosVal - v1*sinVal
		vec[2*i+1] = v0*sinVal + v1*cosVal

		i = i + 1
	}
	return vec
}

// ApplyRoPEVecOff applies Rotary Position Embedding in-place at an offset
func ApplyRoPEVecOff(vec []float64, off int, headDim int, pos int, freqBase float64) []float64 {
	halfDim := headDim / 2
	i := 0
	for i < halfDim {
		freq := 1.0 / Pow(freqBase, float64(2*i)/float64(headDim))
		angle := float64(pos) * freq
		cosVal := Cos(angle)
		sinVal := Sin(angle)

		v0 := vec[off+2*i]
		v1 := vec[off+2*i+1]
		vec[off+2*i] = v0*cosVal - v1*sinVal
		vec[off+2*i+1] = v0*sinVal + v1*cosVal

		i = i + 1
	}
	return vec
}

// AttentionSingleHead computes attention for a single head using offsets
func AttentionSingleHead(q []float64, qOff int, cacheK []float64, cacheV []float64, layer int, headDim int, kvHeadIdx int, seqLen int, maxSeqLen int, kvDim int) []float64 {
	scores := make([]float64, seqLen)
	scale := 1.0 / Sqrt(float64(headDim))
	si := 0
	for si < seqLen {
		kOff := layer*maxSeqLen*kvDim + si*kvDim + kvHeadIdx*headDim
		scores[si] = VecDotOff(q, qOff, cacheK, kOff, headDim) * scale
		si = si + 1
	}

	scores = SoftmaxInPlace(scores, seqLen)

	result := make([]float64, headDim)
	vi := 0
	for vi < seqLen {
		vOff := layer*maxSeqLen*kvDim + vi*kvDim + kvHeadIdx*headDim
		di := 0
		for di < headDim {
			result[di] = result[di] + scores[vi]*cacheV[vOff+di]
			di = di + 1
		}
		vi = vi + 1
	}
	return result
}

// MultiHeadAttention performs multi-head attention for a single layer
func MultiHeadAttention(xnorm []float64, cfg ModelConfig, layer int, cache KVCache, pos int, file GGUFFile, tCache TensorCache) []float64 {
	dim := cfg.EmbeddingLength
	headDim := dim / cfg.HeadCount
	kvHeads := cfg.HeadCountKV
	kvDim := kvHeads * headDim
	headsPerKV := cfg.HeadCount / kvHeads

	qIdx := FindTensorByName(file, layerTensorName(layer, "attn_q.weight"))
	kIdx := FindTensorByName(file, layerTensorName(layer, "attn_k.weight"))
	vIdx := FindTensorByName(file, layerTensorName(layer, "attn_v.weight"))
	oIdx := FindTensorByName(file, layerTensorName(layer, "attn_output.weight"))

	// Compute Q, K, V projections using offset-based MatVecMul
	qOff := CacheOffset(tCache, qIdx)
	qProj := MatVecMulOff(tCache.Entries, qOff, xnorm, dim, dim)

	kOff := CacheOffset(tCache, kIdx)
	kProj := MatVecMulOff(tCache.Entries, kOff, xnorm, kvDim, dim)

	vOff := CacheOffset(tCache, vIdx)
	vProj := MatVecMulOff(tCache.Entries, vOff, xnorm, kvDim, dim)

	// Apply RoPE to Q heads in-place (no copy)
	hi := 0
	for hi < cfg.HeadCount {
		qProj = ApplyRoPEVecOff(qProj, hi*headDim, headDim, pos, cfg.RopeFreqBase)
		hi = hi + 1
	}

	// Apply RoPE to K heads in-place (no copy)
	ki := 0
	for ki < kvHeads {
		kProj = ApplyRoPEVecOff(kProj, ki*headDim, headDim, pos, cfg.RopeFreqBase)
		ki = ki + 1
	}

	// Store K and V in cache
	cache = SetKV(cache, layer, pos, kProj, vProj)

	// Compute attention for each head (pass qProj+offset, no headQ copy)
	attnOut := make([]float64, dim)
	hi2 := 0
	for hi2 < cfg.HeadCount {
		kvIdx := hi2 / headsPerKV
		headOut := AttentionSingleHead(qProj, hi2*headDim, cache.K, cache.V, layer, headDim, kvIdx, pos+1, cache.MaxSeqLen, cache.KVDim)
		di2 := 0
		for di2 < headDim {
			attnOut[hi2*headDim+di2] = headOut[di2]
			di2 = di2 + 1
		}
		hi2 = hi2 + 1
	}

	// Output projection using offset-based MatVecMul
	oOff := CacheOffset(tCache, oIdx)
	projected := MatVecMulOff(tCache.Entries, oOff, attnOut, dim, dim)
	return projected
}
