package anyllm

import "runtime/math"

// Abs returns the absolute value of x
func Abs(x float64) float64 {
	if x < 0.0 {
		return -x
	}
	return x
}

// Floor returns the largest integer <= x
func Floor(x float64) float64 {
	if x >= 0.0 {
		return float64(int64(x))
	}
	n := int64(x)
	if float64(n) == x {
		return x
	}
	return float64(n - 1)
}

// Fmod returns x mod y
func Fmod(x float64, y float64) float64 {
	q := Floor(x / y)
	return x - q*y
}

// Pi constant
const PiVal float64 = 3.14159265358979323846

// Exp computes e^x using range reduction and Taylor series
func Exp(x float64) float64 {
	if x > 709.0 {
		return 1.0e308
	}
	if x < -709.0 {
		return 0.0
	}
	// Range reduction: e^x = 2^k * e^r where r = x - k*ln2
	ln2 := 0.6931471805599453
	k := Floor(x/ln2 + 0.5)
	r := x - k*ln2

	// Taylor series for e^r
	term := 1.0
	sum := 1.0
	i := 1
	for i <= 25 {
		term = term * r / float64(i)
		sum = sum + term
		i = i + 1
	}

	// Multiply by 2^k
	result := sum * pow2(k)
	return result
}

// Sqrt computes square root using Newton's method
func Sqrt(x float64) float64 {
	if x <= 0.0 {
		return 0.0
	}
	guess := x
	if guess > 1.0 {
		guess = guess / 2.0
	}
	i := 0
	for i < 64 {
		guess = (guess + x/guess) / 2.0
		i = i + 1
	}
	return guess
}

// Sin computes sine using Taylor series with range reduction
func Sin(x float64) float64 {
	// Range reduce to [-pi, pi]
	twoPi := 2.0 * PiVal
	x = Fmod(x, twoPi)
	if x > PiVal {
		x = x - twoPi
	} else if x < -PiVal {
		x = x + twoPi
	}

	// Taylor series: sin(x) = x - x^3/3! + x^5/5! - ...
	term := x
	sum := x
	x2 := x * x
	i := 1
	for i <= 12 {
		n := 2*i + 1
		term = -term * x2 / float64(n*(n-1))
		sum = sum + term
		i = i + 1
	}
	return sum
}

// Cos computes cosine using Taylor series with range reduction
func Cos(x float64) float64 {
	// Range reduce to [-pi, pi]
	twoPi := 2.0 * PiVal
	x = Fmod(x, twoPi)
	if x > PiVal {
		x = x - twoPi
	} else if x < -PiVal {
		x = x + twoPi
	}

	// Taylor series: cos(x) = 1 - x^2/2! + x^4/4! - ...
	term := 1.0
	sum := 1.0
	x2 := x * x
	i := 1
	for i <= 12 {
		n := 2 * i
		term = -term * x2 / float64(n*(n-1))
		sum = sum + term
		i = i + 1
	}
	return sum
}

// Pow computes b^e for positive b using Exp(e * Ln(b))
func Pow(b float64, exponent float64) float64 {
	if exponent == 0.0 {
		return 1.0
	}
	if b == 0.0 {
		return 0.0
	}
	if b == 1.0 {
		return 1.0
	}
	lnB := Ln(b)
	return Exp(exponent * lnB)
}

// Ln computes natural logarithm using series expansion
func Ln(x float64) float64 {
	if x <= 0.0 {
		return -1.0e308
	}
	// Normalize to [0.5, 1.5) range
	// x = m * 2^e, ln(x) = ln(m) + e * ln(2)
	ln2 := 0.6931471805599453
	e := 0.0
	for x > 2.0 {
		x = x / 2.0
		e = e + 1.0
	}
	for x < 0.5 {
		x = x * 2.0
		e = e - 1.0
	}
	// Series for ln(1+u) where u = x-1, |u| < 1
	u := x - 1.0
	term := u
	sum := u
	i := 2
	for i <= 40 {
		term = -term * u * float64(i-1) / float64(i)
		sum = sum + term
		i = i + 1
	}
	return sum + e*ln2
}

// VecDot computes dot product of two vectors
func VecDot(a []float64, b []float64, n int) float64 {
	return math.VecDot(a, b, n)
}

// MatVecMul multiplies a matrix (row-major) by a vector: result[i] = dot(mat[i*cols..], vec)
func MatVecMul(mat []float64, vec []float64, rows int, cols int) []float64 {
	return math.MatVecMul(mat, vec, rows, cols)
}

// RMSNorm applies RMS normalization: x[i] * weight[i] / sqrt(mean(x^2) + eps)
func RMSNorm(x []float64, weight []float64, n int, eps float64) []float64 {
	return math.RMSNorm(x, weight, n, eps)
}

// Softmax applies softmax normalization to a vector
func Softmax(x []float64, n int) []float64 {
	return math.Softmax(x, n)
}

// SiLU activation: x * sigmoid(x) = x / (1 + exp(-x))
func SiLU(x float64) float64 {
	return x / (1.0 + Exp(-x))
}

// VecAdd adds two vectors element-wise
func VecAdd(a []float64, b []float64, n int) []float64 {
	return math.VecAdd(a, b, n)
}

// VecMul multiplies two vectors element-wise
func VecMul(a []float64, b []float64, n int) []float64 {
	return math.VecMul(a, b, n)
}

// MatVecMulOff multiplies a matrix at offset (row-major) by a vector
func MatVecMulOff(mat []float64, matOff int, vec []float64, rows int, cols int) []float64 {
	return math.MatVecMulOff(mat, matOff, vec, rows, cols)
}

// VecDotOff computes dot product with offsets into both arrays
func VecDotOff(a []float64, aOff int, b []float64, bOff int, n int) float64 {
	return math.VecDotOff(a, aOff, b, bOff, n)
}

// SoftmaxInPlace applies softmax normalization in-place
func SoftmaxInPlace(x []float64, n int) []float64 {
	maxVal := x[0]
	i := 1
	for i < n {
		if x[i] > maxVal {
			maxVal = x[i]
		}
		i = i + 1
	}
	sum := 0.0
	j := 0
	for j < n {
		x[j] = Exp(x[j] - maxVal)
		sum = sum + x[j]
		j = j + 1
	}
	invSum := 1.0 / sum
	k := 0
	for k < n {
		x[k] = x[k] * invSum
		k = k + 1
	}
	return x
}

// ArgMax returns the index of the maximum value in a vector
func ArgMax(x []float64, n int) int {
	maxIdx := 0
	maxVal := x[0]
	i := 1
	for i < n {
		if x[i] > maxVal {
			maxVal = x[i]
			maxIdx = i
		}
		i = i + 1
	}
	return maxIdx
}
