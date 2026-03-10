package anyllm

// FFNSingleExpert computes SwiGLU FFN for a single expert
func FFNSingleExpert(xnorm []float64, cfg ModelConfig, layer int, expertIdx int, file GGUFFile, tCache TensorCache) []float64 {
	dim := cfg.EmbeddingLength
	ffnDim := cfg.FFNLength

	layerStr := intToString(layer)
	expertStr := intToString(expertIdx)

	gateName := "blk."
	gateName += layerStr
	gateName += ".ffn_gate."
	gateName += expertStr
	gateName += ".weight"
	gateIdx := FindTensorByName(file, gateName)

	upName := "blk."
	upName += layerStr
	upName += ".ffn_up."
	upName += expertStr
	upName += ".weight"
	upIdx := FindTensorByName(file, upName)

	downName := "blk."
	downName += layerStr
	downName += ".ffn_down."
	downName += expertStr
	downName += ".weight"
	downIdx := FindTensorByName(file, downName)

	gateWeights, _ := ReadTensorCached(file, gateIdx, tCache)
	gateOut := MatVecMul(gateWeights, xnorm, ffnDim, dim)

	upWeights, _ := ReadTensorCached(file, upIdx, tCache)
	upOut := MatVecMul(upWeights, xnorm, ffnDim, dim)

	hidden := make([]float64, ffnDim)
	i := 0
	for i < ffnDim {
		hidden[i] = SiLU(gateOut[i]) * upOut[i]
		i = i + 1
	}

	downWeights, _ := ReadTensorCached(file, downIdx, tCache)
	result := MatVecMul(downWeights, hidden, dim, ffnDim)
	return result
}

// FFN computes the SwiGLU feed-forward network for a single layer (non-MoE)
func FFN(xnorm []float64, cfg ModelConfig, layer int, file GGUFFile, tCache TensorCache) []float64 {
	dim := cfg.EmbeddingLength
	ffnDim := cfg.FFNLength

	gateIdx := FindTensorByName(file, layerTensorName(layer, "ffn_gate.weight"))
	upIdx := FindTensorByName(file, layerTensorName(layer, "ffn_up.weight"))
	downIdx := FindTensorByName(file, layerTensorName(layer, "ffn_down.weight"))

	tCache = TensorCacheLoadFFN(tCache, file, gateIdx, tCache.FFNSlot1)
	gateOut := MatVecMulOff(tCache.Entries, tCache.FFNSlot1, xnorm, ffnDim, dim)

	tCache = TensorCacheLoadFFN(tCache, file, upIdx, tCache.FFNSlot2)
	upOut := MatVecMulOff(tCache.Entries, tCache.FFNSlot2, xnorm, ffnDim, dim)

	hidden := make([]float64, ffnDim)
	i := 0
	for i < ffnDim {
		hidden[i] = SiLU(gateOut[i]) * upOut[i]
		i = i + 1
	}

	tCache = TensorCacheLoadFFN(tCache, file, downIdx, tCache.FFNSlot3)
	result := MatVecMulOff(tCache.Entries, tCache.FFNSlot3, hidden, dim, ffnDim)
	return result
}

// MoEFFN computes Mixture-of-Experts FFN
// Routes input to top-k experts via a gating network, combines their outputs
func MoEFFN(xnorm []float64, cfg ModelConfig, layer int, file GGUFFile, tCache TensorCache) []float64 {
	dim := cfg.EmbeddingLength
	numExperts := cfg.ExpertCount
	topK := cfg.ExpertUsedCount

	// Load routing gate
	routeIdx := FindTensorByName(file, layerTensorName(layer, "ffn_gate_inp.weight"))
	routeWeights, _ := ReadTensorCached(file, routeIdx, tCache)
	expertScores := MatVecMul(routeWeights, xnorm, numExperts, dim)

	// Softmax over expert scores
	expertProbs := Softmax(expertScores, numExperts)

	// Find top-k experts
	topExperts := make([]int, topK)
	topWeights := make([]float64, topK)
	ti := 0
	for ti < topK {
		bestIdx := -1
		bestVal := -1.0e30
		ej := 0
		for ej < numExperts {
			// Check if already selected
			alreadyPicked := false
			sj := 0
			for sj < ti {
				if topExperts[sj] == ej {
					alreadyPicked = true
				}
				sj = sj + 1
			}
			if !alreadyPicked && expertProbs[ej] > bestVal {
				bestVal = expertProbs[ej]
				bestIdx = ej
			}
			ej = ej + 1
		}
		topExperts[ti] = bestIdx
		topWeights[ti] = bestVal
		ti = ti + 1
	}

	// Renormalize weights of selected experts
	weightSum := 0.0
	wi := 0
	for wi < topK {
		weightSum = weightSum + topWeights[wi]
		wi = wi + 1
	}
	wi2 := 0
	for wi2 < topK {
		topWeights[wi2] = topWeights[wi2] / weightSum
		wi2 = wi2 + 1
	}

	// Compute weighted sum of expert outputs
	result := make([]float64, dim)
	ei := 0
	for ei < topK {
		expertOut := FFNSingleExpert(xnorm, cfg, layer, topExperts[ei], file, tCache)
		w := topWeights[ei]
		di := 0
		for di < dim {
			result[di] = result[di] + w*expertOut[di]
			di = di + 1
		}
		ei = ei + 1
	}
	return result
}
