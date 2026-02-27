#ifndef MATH_SIMD_H
#define MATH_SIMD_H

#ifdef __cplusplus
extern "C" {
#endif

void simd_vec_dot(const double* a, const double* b, int n, double* result);
void simd_mat_vec_mul(const double* mat, const double* vec, int rows, int cols, double* out);
void simd_vec_add(const double* a, const double* b, int n, double* out);
void simd_vec_mul(const double* a, const double* b, int n, double* out);
void simd_rms_norm(const double* x, const double* weight, int n, double eps, double* out);
void simd_softmax(double* x, int n);
void simd_silu_vec(double* x, int n);
void simd_mat_vec_mul_off(const double* mat, int mat_off, const double* vec, int rows, int cols, double* out);
void simd_vec_dot_off(const double* a, int a_off, const double* b, int b_off, int n, double* result);

#ifdef __cplusplus
}
#endif

#endif /* MATH_SIMD_H */
