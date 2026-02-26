package anyllm

// Tokenizer holds BPE tokenizer data
type Tokenizer struct {
	Vocab     []string
	Scores    []float64
	VocabSize int
	BosID     int
	EosID     int
	SortedIdx []int
}

// strCmp compares two strings lexicographically
// Returns -1 if a < b, 0 if a == b, 1 if a > b
func strCmp(a string, b string) int {
	la := len(a)
	lb := len(b)
	minLen := la
	if lb < minLen {
		minLen = lb
	}
	i := 0
	for i < minLen {
		ca := int(a[i])
		cb := int(b[i])
		if ca < cb {
			return -1
		} else if ca > cb {
			return 1
		}
		i = i + 1
	}
	if la < lb {
		return -1
	} else if la > lb {
		return 1
	}
	return 0
}

// LoadTokenizer loads tokenizer data from GGUF metadata
func LoadTokenizer(file GGUFFile) Tokenizer {
	tok := Tokenizer{}

	vocab := ReadMetadataStringArray(file, "tokenizer.ggml.tokens")
	tok.Vocab = vocab
	tok.VocabSize = len(vocab)

	scores := ReadMetadataFloatArray(file, "tokenizer.ggml.scores")
	tok.Scores = scores

	tok.BosID = int(GetMetadataInt(file, "tokenizer.ggml.bos_token_id", 1))
	tok.EosID = int(GetMetadataInt(file, "tokenizer.ggml.eos_token_id", 2))

	// Build sorted index for binary search lookup
	tok.SortedIdx = buildSortedIndex(vocab)

	return tok
}

// buildSortedIndex creates an array of indices sorted by vocab string
func buildSortedIndex(vocab []string) []int {
	n := len(vocab)
	idx := make([]int, n)
	i := 0
	for i < n {
		idx[i] = i
		i = i + 1
	}
	// Insertion sort (simple, no recursion needed)
	j := 1
	for j < n {
		key := idx[j]
		keyStr := vocab[key]
		k := j - 1
		for k >= 0 && strCmp(vocab[idx[k]], keyStr) > 0 {
			idx[k+1] = idx[k]
			k = k - 1
		}
		idx[k+1] = key
		j = j + 1
	}
	return idx
}

// vocabLookup finds a token string in the vocabulary using binary search
// Returns the token ID or -1 if not found
func vocabLookup(tok Tokenizer, text string) int {
	lo := 0
	hi := tok.VocabSize - 1
	for lo <= hi {
		mid := (lo + hi) / 2
		midStr := tok.Vocab[tok.SortedIdx[mid]]
		cmp := strCmp(midStr, text)
		if cmp == 0 {
			return tok.SortedIdx[mid]
		} else if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return -1
}

// Encode tokenizes a string using BPE
func Encode(tok Tokenizer, text string) []int {
	if len(text) == 0 {
		empty := []int{}
		return empty
	}

	// Start with one token per character (byte-level)
	tokens := []int{}
	ci := 0
	for ci < len(text) {
		ch := int(text[ci])
		s := charToString(ch)
		tid := vocabLookup(tok, s)
		if tid >= 0 {
			tokens = append(tokens, tid)
		} else {
			// Unknown byte - use 0
			tokens = append(tokens, 0)
		}
		ci = ci + 1
	}

	// BPE merge loop
	for len(tokens) >= 2 {
		// Find the pair with the highest score
		bestScore := -1.0e30
		bestIdx := -1
		bestID := -1
		pi := 0
		for pi < len(tokens)-1 {
			leftTok := tokens[pi]
			rightTok := tokens[pi+1]
			merged := tok.Vocab[leftTok]
			merged += tok.Vocab[rightTok]
			mid := vocabLookup(tok, merged)
			if mid >= 0 {
				sc := tok.Scores[mid]
				if sc > bestScore {
					bestScore = sc
					bestIdx = pi
					bestID = mid
				}
			}
			pi = pi + 1
		}
		if bestIdx < 0 {
			break
		}
		// Merge the best pair
		newTokens := []int{}
		ti := 0
		for ti < len(tokens) {
			if ti == bestIdx {
				newTokens = append(newTokens, bestID)
				ti = ti + 2
			} else {
				newTokens = append(newTokens, tokens[ti])
				ti = ti + 1
			}
		}
		tokens = newTokens
	}

	// Prepend BOS token
	output := []int{}
	output = append(output, tok.BosID)
	ri := 0
	for ri < len(tokens) {
		output = append(output, tokens[ri])
		ri = ri + 1
	}
	return output
}

// Decode converts token IDs back to a string
func Decode(tok Tokenizer, tokens []int) string {
	decoded := ""
	i := 0
	for i < len(tokens) {
		tid := tokens[i]
		if tid >= 0 && tid < tok.VocabSize {
			decoded += tok.Vocab[tid]
		}
		i = i + 1
	}
	return decoded
}

// DecodeToken converts a single token ID to a string
func DecodeToken(tok Tokenizer, tokenID int) string {
	if tokenID >= 0 && tokenID < tok.VocabSize {
		return tok.Vocab[tokenID]
	}
	return ""
}
