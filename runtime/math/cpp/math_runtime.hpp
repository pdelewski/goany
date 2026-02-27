// math_runtime.hpp - SIMD math runtime for goany math package
// Links against math_simd.c for NEON/AVX2 acceleration

#ifndef MATH_RUNTIME_HPP
#define MATH_RUNTIME_HPP

#include "../c/math_simd.h"
#include <vector>
#include <cmath>

namespace math {

inline double VecDot(const std::vector<double>& a, const std::vector<double>& b, int n) {
    double result = 0.0;
    simd_vec_dot(a.data(), b.data(), n, &result);
    return result;
}

inline std::vector<double> MatVecMul(const std::vector<double>& mat, const std::vector<double>& vec, int rows, int cols) {
    std::vector<double> out(rows, 0.0);
    simd_mat_vec_mul(mat.data(), vec.data(), rows, cols, out.data());
    return out;
}

inline std::vector<double> VecAdd(const std::vector<double>& a, const std::vector<double>& b, int n) {
    std::vector<double> out(n, 0.0);
    simd_vec_add(a.data(), b.data(), n, out.data());
    return out;
}

inline std::vector<double> VecMul(const std::vector<double>& a, const std::vector<double>& b, int n) {
    std::vector<double> out(n, 0.0);
    simd_vec_mul(a.data(), b.data(), n, out.data());
    return out;
}

inline std::vector<double> RMSNorm(const std::vector<double>& x, const std::vector<double>& weight, int n, double eps) {
    std::vector<double> out(n, 0.0);
    simd_rms_norm(x.data(), weight.data(), n, eps, out.data());
    return out;
}

inline std::vector<double> Softmax(const std::vector<double>& x, int n) {
    std::vector<double> out(x.begin(), x.begin() + n);
    out.resize(n);
    simd_softmax(out.data(), n);
    return out;
}

inline std::vector<double> SiLUVec(const std::vector<double>& x, int n) {
    std::vector<double> out(x.begin(), x.begin() + n);
    out.resize(n);
    simd_silu_vec(out.data(), n);
    return out;
}

inline std::vector<double> MatVecMulOff(const std::vector<double>& mat, int matOff, const std::vector<double>& vec, int rows, int cols) {
    std::vector<double> out(rows, 0.0);
    simd_mat_vec_mul_off(mat.data(), matOff, vec.data(), rows, cols, out.data());
    return out;
}

inline double VecDotOff(const std::vector<double>& a, int aOff, const std::vector<double>& b, int bOff, int n) {
    double result = 0.0;
    simd_vec_dot_off(a.data(), aOff, b.data(), bOff, n, &result);
    return result;
}

} // namespace math

#endif // MATH_RUNTIME_HPP
