package anyllm

// TransformerBlock runs a single transformer block (layer)
func TransformerBlock(x []float64, cfg ModelConfig, layer int, cache KVCache, pos int, file GGUFFile, tCache TensorCache) []float64 {
	dim := cfg.EmbeddingLength

	// Load attention norm weights
	attnNormIdx := FindTensorByName(file, layerTensorName(layer, "attn_norm.weight"))
	attnNormW := ReadTensorCached(file, attnNormIdx, tCache)

	// Pre-attention RMS norm
	xnorm := RMSNorm(x, attnNormW, dim, cfg.RMSNormEps)

	// Multi-head attention
	attnOut := MultiHeadAttention(xnorm, cfg, layer, cache, pos, file, tCache)

	// Residual connection
	h := VecAdd(x, attnOut, dim)

	// Load FFN norm weights
	ffnNormIdx := FindTensorByName(file, layerTensorName(layer, "ffn_norm.weight"))
	ffnNormW := ReadTensorCached(file, ffnNormIdx, tCache)

	// Pre-FFN RMS norm
	hnorm := RMSNorm(h, ffnNormW, dim, cfg.RMSNormEps)

	// FFN (MoE or standard)
	ffnOut := make([]float64, dim)
	if cfg.ExpertCount > 0 {
		ffnOut = MoEFFN(hnorm, cfg, layer, file, tCache)
	} else {
		ffnOut = FFN(hnorm, cfg, layer, file, tCache)
	}

	// Residual connection
	result := VecAdd(h, ffnOut, dim)
	return result
}

// Forward runs the complete forward pass of the transformer
func Forward(file GGUFFile, cfg ModelConfig, cache KVCache, tCache TensorCache, tokenID int, pos int) []float64 {
	dim := cfg.EmbeddingLength

	// Token embedding lookup
	embIdx := FindTensorByName(file, "token_embd.weight")
	embWeights := ReadTensorCached(file, embIdx, tCache)
	x := make([]float64, dim)
	offset := tokenID * dim
	i := 0
	for i < dim {
		x[i] = embWeights[offset+i]
		i = i + 1
	}

	// Run through all transformer blocks
	layer := 0
	for layer < cfg.BlockCount {
		x = TransformerBlock(x, cfg, layer, cache, pos, file, tCache)
		layer = layer + 1
	}

	// Final RMS norm
	normIdx := FindTensorByName(file, "output_norm.weight")
	normW := ReadTensorCached(file, normIdx, tCache)
	x = RMSNorm(x, normW, dim, cfg.RMSNormEps)

	// Output projection to logits
	outIdx := FindTensorByName(file, "output.weight")
	if outIdx < 0 {
		// Some models tie output weights to embedding
		outIdx = embIdx
	}
	outWeights := ReadTensorCached(file, outIdx, tCache)
	logits := MatVecMul(outWeights, x, cfg.VocabSize, dim)
	return logits
}
