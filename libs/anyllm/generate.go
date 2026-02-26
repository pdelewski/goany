package anyllm

// Generate generates text using the LLM model
func Generate(file GGUFFile, cfg ModelConfig, tok Tokenizer, prompt string, maxTokens int) string {
	// Tokenize prompt
	promptTokens := Encode(tok, prompt)

	// Create KV cache
	kvDim := cfg.HeadCountKV * (cfg.EmbeddingLength / cfg.HeadCount)
	cache := NewKVCache(cfg.ContextLength, cfg.BlockCount, kvDim)

	// Create tensor cache and prewarm with non-expert weights
	tCache := NewTensorCache(file, cfg)
	PrewarmCache(file, cfg, tCache)

	// Prefill: process all prompt tokens
	pos := 0
	nextToken := 0
	pi := 0
	for pi < len(promptTokens) {
		logits := Forward(file, cfg, cache, tCache, promptTokens[pi], pos)
		nextToken = ArgMax(logits, cfg.VocabSize)
		pos = pos + 1
		pi = pi + 1
	}

	// Generate output tokens
	generated := []int{}
	gi := 0
	for gi < maxTokens {
		if nextToken == tok.EosID {
			break
		}
		generated = append(generated, nextToken)
		logits := Forward(file, cfg, cache, tCache, nextToken, pos)
		nextToken = ArgMax(logits, cfg.VocabSize)
		pos = pos + 1
		gi = gi + 1
	}

	result := Decode(tok, generated)
	return result
}
