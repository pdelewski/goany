package anyllm

// ExtractModelConfig extracts model configuration from GGUF metadata
func ExtractModelConfig(file GGUFFile) ModelConfig {
	cfg := ModelConfig{}

	arch := GetMetadataString(file, "general.architecture", "")
	if arch == "" {
		cfg.Error = "missing general.architecture metadata"
		return cfg
	}
	cfg.Architecture = arch

	embKey := arch
	embKey += ".embedding_length"
	cfg.EmbeddingLength = int(GetMetadataInt(file, embKey, 0))

	blockKey := arch
	blockKey += ".block_count"
	cfg.BlockCount = int(GetMetadataInt(file, blockKey, 0))

	headKey := arch
	headKey += ".attention.head_count"
	cfg.HeadCount = int(GetMetadataInt(file, headKey, 0))

	headKVKey := arch
	headKVKey += ".attention.head_count_kv"
	cfg.HeadCountKV = int(GetMetadataInt(file, headKVKey, 0))
	if cfg.HeadCountKV == 0 {
		cfg.HeadCountKV = cfg.HeadCount
	}

	ctxKey := arch
	ctxKey += ".context_length"
	cfg.ContextLength = int(GetMetadataInt(file, ctxKey, 2048))

	ffnKey := arch
	ffnKey += ".feed_forward_length"
	cfg.FFNLength = int(GetMetadataInt(file, ffnKey, 0))

	ropeKey := arch
	ropeKey += ".rope.freq_base"
	cfg.RopeFreqBase = GetMetadataFloat(file, ropeKey, 10000.0)

	epsKey := arch
	epsKey += ".attention.layer_norm_rms_epsilon"
	cfg.RMSNormEps = GetMetadataFloat(file, epsKey, 0.00001)

	expertKey := arch
	expertKey += ".expert_count"
	cfg.ExpertCount = int(GetMetadataInt(file, expertKey, 0))

	expertUsedKey := arch
	expertUsedKey += ".expert_used_count"
	cfg.ExpertUsedCount = int(GetMetadataInt(file, expertUsedKey, 0))

	// Vocab size from tokenizer metadata
	vocabIdx := FindMetadata(file, "tokenizer.ggml.tokens")
	if vocabIdx >= 0 {
		cfg.VocabSize = int(file.Metadata[vocabIdx].ArrayLen)
	}

	if cfg.EmbeddingLength == 0 {
		cfg.Error = "missing embedding_length"
	} else if cfg.BlockCount == 0 {
		cfg.Error = "missing block_count"
	} else if cfg.HeadCount == 0 {
		cfg.Error = "missing head_count"
	}

	return cfg
}

// FindTensorByName finds a tensor by name and returns its index, or -1 if not found
func FindTensorByName(file GGUFFile, name string) int {
	i := 0
	for i < len(file.Tensors) {
		if file.Tensors[i].Name == name {
			return i
		}
		i = i + 1
	}
	return -1
}
