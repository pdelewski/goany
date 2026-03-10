package anyllm

// RNGState holds the state of a linear congruential generator
type RNGState struct {
	State int64
}

// NewRNG creates a new RNG with the given seed
func NewRNG(seed int64) RNGState {
	rng := RNGState{}
	rng.State = seed
	return rng
}

// NextRNG advances the RNG and returns the next value
func NextRNG(rng RNGState) (RNGState, int) {
	// LCG: state = (a * state + c) mod 2^31
	a := int64(1103515245)
	c := int64(12345)
	rng.State = (a*rng.State + c) & 2147483647
	val := int(rng.State)
	return rng, val
}

// RandFloat returns a random float64 in [0.0, 1.0)
func RandFloat(rng RNGState) (RNGState, float64) {
	rng2, val := NextRNG(rng)
	f := float64(val) / 2147483647.0
	return rng2, f
}

// ApplyTemperature scales logits by temperature in-place
func ApplyTemperature(logits []float64, n int, temperature float64) []float64 {
	i := 0
	for i < n {
		logits[i] = logits[i] / temperature
		i = i + 1
	}
	return logits
}

// SampleTopP samples from the top-p (nucleus) of the probability distribution
// Uses iterative max-extraction approach (no sorting needed)
func SampleTopP(logits []float64, n int, topP float64, rng RNGState) ([]float64, int, RNGState) {
	// First apply softmax to get probabilities
	logits = SoftmaxInPlace(logits, n)

	// Iteratively extract the highest-probability tokens until cumulative >= topP
	// Track which indices have been "used" by setting them to -1.0
	cumProb := 0.0
	selectedCount := 0
	selectedIdx := make([]int, n)
	selectedProb := make([]float64, n)

	for cumProb < topP && selectedCount < n {
		// Find the max probability among remaining
		bestIdx := -1
		bestVal := -1.0
		j := 0
		for j < n {
			if logits[j] > bestVal {
				bestVal = logits[j]
				bestIdx = j
			}
			j = j + 1
		}
		if bestIdx < 0 {
			break
		}
		selectedIdx[selectedCount] = bestIdx
		selectedProb[selectedCount] = bestVal
		cumProb = cumProb + bestVal
		logits[bestIdx] = -1.0 // mark as used
		selectedCount = selectedCount + 1
	}

	if selectedCount == 0 {
		return logits, 0, rng
	}

	// Renormalize selected probabilities
	normSum := 0.0
	ni := 0
	for ni < selectedCount {
		normSum = normSum + selectedProb[ni]
		ni = ni + 1
	}

	// Sample from the selected tokens
	rng2, r := RandFloat(rng)
	r = r * normSum
	cumul := 0.0
	si := 0
	for si < selectedCount {
		cumul = cumul + selectedProb[si]
		if cumul > r {
			return logits, selectedIdx[si], rng2
		}
		si = si + 1
	}
	// Fallback to last selected token
	return logits, selectedIdx[selectedCount-1], rng2
}

// SampleToken samples a token from logits using temperature and top-p
// When temperature <= 0, uses greedy ArgMax decoding
func SampleToken(logits []float64, n int, temperature float64, topP float64, rng RNGState) (int, RNGState) {
	if temperature <= 0.0 {
		idx := ArgMax(logits, n)
		return idx, rng
	}
	logits = ApplyTemperature(logits, n, temperature)
	_, token, rng2 := SampleTopP(logits, n, topP, rng)
	return token, rng2
}
