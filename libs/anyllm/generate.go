package anyllm

// DefaultGenerateParams returns default generation parameters
func DefaultGenerateParams(chatFormat int) GenerateParams {
	gp := GenerateParams{}
	gp.Temperature = 0.7
	gp.TopP = 0.9
	gp.ChatFormat = chatFormat
	gp.Seed = 42
	return gp
}

// GenerateWithParams generates text using the LLM model with sampling parameters
func GenerateWithParams(file GGUFFile, cfg ModelConfig, tok Tokenizer, prompt string, maxTokens int, gparams GenerateParams) string {
	// Apply chat template
	formattedPrompt := ApplyChatTemplate(prompt, gparams.ChatFormat)

	// Tokenize prompt
	promptTokens := Encode(tok, formattedPrompt)

	// Create KV cache
	kvDim := cfg.HeadCountKV * (cfg.EmbeddingLength / cfg.HeadCount)
	cache := NewKVCache(cfg.ContextLength, cfg.BlockCount, kvDim)

	// Create tensor cache and prewarm with non-expert weights
	tCache := NewTensorCache(file, cfg)
	PrewarmCache(file, cfg, tCache)

	// Initialize RNG
	rng := NewRNG(gparams.Seed)

	// Prefill: process all prompt tokens
	pos := 0
	nextToken := 0
	pi := 0
	for pi < len(promptTokens) {
		logits := Forward(file, cfg, cache, tCache, promptTokens[pi], pos)
		nextToken, rng = SampleToken(logits, cfg.VocabSize, gparams.Temperature, gparams.TopP, rng)
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
		nextToken, rng = SampleToken(logits, cfg.VocabSize, gparams.Temperature, gparams.TopP, rng)
		pos = pos + 1
		gi = gi + 1
	}

	result := Decode(tok, generated)
	return result
}

// Generate generates text using the LLM model with greedy decoding (backward compatible)
func Generate(file GGUFFile, cfg ModelConfig, tok Tokenizer, prompt string, maxTokens int) string {
	gparams := GenerateParams{}
	gparams.Temperature = 0.0
	gparams.TopP = 1.0
	gparams.ChatFormat = ChatFormatNone
	gparams.Seed = 0
	return GenerateWithParams(file, cfg, tok, prompt, maxTokens, gparams)
}
