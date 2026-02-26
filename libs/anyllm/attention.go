package anyllm

// ApplyRoPEVec applies Rotary Position Embedding to a single vector in-place
func ApplyRoPEVec(vec []float64, headDim int, pos int, freqBase float64) {
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
}

// AttentionSingleHead computes attention for a single head
func AttentionSingleHead(q []float64, cache KVCache, layer int, headDim int, kvHeadIdx int, seqLen int) []float64 {
	scores := make([]float64, seqLen)
	scale := 1.0 / Sqrt(float64(headDim))
	si := 0
	for si < seqLen {
		kVec := GetK(cache, layer, si)
		dot := 0.0
		di := 0
		for di < headDim {
			dot = dot + q[di]*kVec[kvHeadIdx*headDim+di]
			di = di + 1
		}
		scores[si] = dot * scale
		si = si + 1
	}

	scores = Softmax(scores, seqLen)

	result := make([]float64, headDim)
	vi := 0
	for vi < seqLen {
		vVec := GetV(cache, layer, vi)
		di := 0
		for di < headDim {
			result[di] = result[di] + scores[vi]*vVec[kvHeadIdx*headDim+di]
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

	// Compute Q, K, V projections
	qWeights := ReadTensorCached(file, qIdx, tCache)
	qProj := MatVecMul(qWeights, xnorm, dim, dim)

	kWeights := ReadTensorCached(file, kIdx, tCache)
	kProj := MatVecMul(kWeights, xnorm, kvDim, dim)

	vWeights := ReadTensorCached(file, vIdx, tCache)
	vProj := MatVecMul(vWeights, xnorm, kvDim, dim)

	// Apply RoPE to Q heads
	hi := 0
	for hi < cfg.HeadCount {
		qSlice := make([]float64, headDim)
		di := 0
		for di < headDim {
			qSlice[di] = qProj[hi*headDim+di]
			di = di + 1
		}
		ApplyRoPEVec(qSlice, headDim, pos, cfg.RopeFreqBase)
		di2 := 0
		for di2 < headDim {
			qProj[hi*headDim+di2] = qSlice[di2]
			di2 = di2 + 1
		}
		hi = hi + 1
	}

	// Apply RoPE to K heads (each KV head only once)
	ki := 0
	for ki < kvHeads {
		kSlice := make([]float64, headDim)
		di := 0
		for di < headDim {
			kSlice[di] = kProj[ki*headDim+di]
			di = di + 1
		}
		ApplyRoPEVec(kSlice, headDim, pos, cfg.RopeFreqBase)
		di2 := 0
		for di2 < headDim {
			kProj[ki*headDim+di2] = kSlice[di2]
			di2 = di2 + 1
		}
		ki = ki + 1
	}

	// Store K and V in cache
	SetKV(cache, layer, pos, kProj, vProj)

	// Compute attention for each head
	attnOut := make([]float64, dim)
	hi2 := 0
	for hi2 < cfg.HeadCount {
		kvIdx := hi2 / headsPerKV
		headQ := make([]float64, headDim)
		di := 0
		for di < headDim {
			headQ[di] = qProj[hi2*headDim+di]
			di = di + 1
		}
		headOut := AttentionSingleHead(headQ, cache, layer, headDim, kvIdx, pos+1)
		di2 := 0
		for di2 < headDim {
			attnOut[hi2*headDim+di2] = headOut[di2]
			di2 = di2 + 1
		}
		hi2 = hi2 + 1
	}

	// Output projection
	oWeights := ReadTensorCached(file, oIdx, tCache)
	projected := MatVecMul(oWeights, attnOut, dim, dim)
	return projected
}
