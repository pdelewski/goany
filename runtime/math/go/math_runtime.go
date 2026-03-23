package gomath

/*
#cgo CFLAGS: -O3
#cgo linux LDFLAGS: -lm
#cgo windows LDFLAGS: -lm
#include "../c/math_simd.h"
*/
import "C"
import "unsafe"

func VecDot(a []float64, b []float64, n int) float64 {
	var result float64
	C.simd_vec_dot(
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.int(n),
		(*C.double)(unsafe.Pointer(&result)),
	)
	return result
}

func MatVecMul(mat []float64, vec []float64, rows int, cols int) []float64 {
	out := make([]float64, rows)
	C.simd_mat_vec_mul(
		(*C.double)(unsafe.Pointer(&mat[0])),
		(*C.double)(unsafe.Pointer(&vec[0])),
		C.int(rows),
		C.int(cols),
		(*C.double)(unsafe.Pointer(&out[0])),
	)
	return out
}

func VecAdd(a []float64, b []float64, n int) []float64 {
	out := make([]float64, n)
	C.simd_vec_add(
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.int(n),
		(*C.double)(unsafe.Pointer(&out[0])),
	)
	return out
}

func VecMul(a []float64, b []float64, n int) []float64 {
	out := make([]float64, n)
	C.simd_vec_mul(
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.int(n),
		(*C.double)(unsafe.Pointer(&out[0])),
	)
	return out
}

func RMSNorm(x []float64, weight []float64, n int, eps float64) []float64 {
	out := make([]float64, n)
	C.simd_rms_norm(
		(*C.double)(unsafe.Pointer(&x[0])),
		(*C.double)(unsafe.Pointer(&weight[0])),
		C.int(n),
		C.double(eps),
		(*C.double)(unsafe.Pointer(&out[0])),
	)
	return out
}

func Softmax(x []float64, n int) []float64 {
	out := make([]float64, n)
	copy(out, x[:n])
	C.simd_softmax(
		(*C.double)(unsafe.Pointer(&out[0])),
		C.int(n),
	)
	return out
}

func SiLUVec(x []float64, n int) []float64 {
	out := make([]float64, n)
	copy(out, x[:n])
	C.simd_silu_vec(
		(*C.double)(unsafe.Pointer(&out[0])),
		C.int(n),
	)
	return out
}

func MatVecMulOff(mat []float64, matOff int, vec []float64, rows int, cols int) []float64 {
	out := make([]float64, rows)
	C.simd_mat_vec_mul_off(
		(*C.double)(unsafe.Pointer(&mat[0])),
		C.int(matOff),
		(*C.double)(unsafe.Pointer(&vec[0])),
		C.int(rows),
		C.int(cols),
		(*C.double)(unsafe.Pointer(&out[0])),
	)
	return out
}

func VecDotOff(a []float64, aOff int, b []float64, bOff int, n int) float64 {
	var result float64
	C.simd_vec_dot_off(
		(*C.double)(unsafe.Pointer(&a[0])),
		C.int(aOff),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.int(bOff),
		C.int(n),
		(*C.double)(unsafe.Pointer(&result)),
	)
	return result
}
