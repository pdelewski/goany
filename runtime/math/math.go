package math

import gomath "runtime/math/go"

func VecDot(a []float64, b []float64, n int) float64 {
	return gomath.VecDot(a, b, n)
}

func MatVecMul(mat []float64, vec []float64, rows int, cols int) []float64 {
	return gomath.MatVecMul(mat, vec, rows, cols)
}

func VecAdd(a []float64, b []float64, n int) []float64 {
	return gomath.VecAdd(a, b, n)
}

func VecMul(a []float64, b []float64, n int) []float64 {
	return gomath.VecMul(a, b, n)
}

func RMSNorm(x []float64, weight []float64, n int, eps float64) []float64 {
	return gomath.RMSNorm(x, weight, n, eps)
}

func Softmax(x []float64, n int) []float64 {
	return gomath.Softmax(x, n)
}

func SiLUVec(x []float64, n int) []float64 {
	return gomath.SiLUVec(x, n)
}

func MatVecMulOff(mat []float64, matOff int, vec []float64, rows int, cols int) []float64 {
	return gomath.MatVecMulOff(mat, matOff, vec, rows, cols)
}

func VecDotOff(a []float64, aOff int, b []float64, bOff int, n int) float64 {
	return gomath.VecDotOff(a, aOff, b, bOff, n)
}
