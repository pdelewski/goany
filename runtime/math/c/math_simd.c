#include "math_simd.h"
#include <math.h>

/* ------------------------------------------------------------------ */
/* Architecture detection                                              */
/* ------------------------------------------------------------------ */

#if defined(__aarch64__) || defined(_M_ARM64)
  #include <arm_neon.h>
  #define USE_NEON 1
#elif defined(__x86_64__) || defined(_M_X64)
  #if defined(__AVX2__) && defined(__FMA__)
    #include <immintrin.h>
    #define USE_AVX2 1
  #endif
#endif

/* ------------------------------------------------------------------ */
/* simd_vec_dot                                                        */
/* ------------------------------------------------------------------ */

void simd_vec_dot(const double* a, const double* b, int n, double* result) {
    double sum = 0.0;
    int i = 0;

#if USE_NEON
    float64x2_t acc0 = vdupq_n_f64(0.0);
    float64x2_t acc1 = vdupq_n_f64(0.0);
    float64x2_t acc2 = vdupq_n_f64(0.0);
    float64x2_t acc3 = vdupq_n_f64(0.0);
    for (; i + 7 < n; i += 8) {
        acc0 = vfmaq_f64(acc0, vld1q_f64(a + i),     vld1q_f64(b + i));
        acc1 = vfmaq_f64(acc1, vld1q_f64(a + i + 2), vld1q_f64(b + i + 2));
        acc2 = vfmaq_f64(acc2, vld1q_f64(a + i + 4), vld1q_f64(b + i + 4));
        acc3 = vfmaq_f64(acc3, vld1q_f64(a + i + 6), vld1q_f64(b + i + 6));
    }
    acc0 = vaddq_f64(acc0, acc1);
    acc2 = vaddq_f64(acc2, acc3);
    acc0 = vaddq_f64(acc0, acc2);
    sum = vgetq_lane_f64(acc0, 0) + vgetq_lane_f64(acc0, 1);
#elif USE_AVX2
    __m256d acc0 = _mm256_setzero_pd();
    __m256d acc1 = _mm256_setzero_pd();
    __m256d acc2 = _mm256_setzero_pd();
    __m256d acc3 = _mm256_setzero_pd();
    for (; i + 15 < n; i += 16) {
        acc0 = _mm256_fmadd_pd(_mm256_loadu_pd(a + i),      _mm256_loadu_pd(b + i),      acc0);
        acc1 = _mm256_fmadd_pd(_mm256_loadu_pd(a + i + 4),  _mm256_loadu_pd(b + i + 4),  acc1);
        acc2 = _mm256_fmadd_pd(_mm256_loadu_pd(a + i + 8),  _mm256_loadu_pd(b + i + 8),  acc2);
        acc3 = _mm256_fmadd_pd(_mm256_loadu_pd(a + i + 12), _mm256_loadu_pd(b + i + 12), acc3);
    }
    acc0 = _mm256_add_pd(acc0, acc1);
    acc2 = _mm256_add_pd(acc2, acc3);
    acc0 = _mm256_add_pd(acc0, acc2);
    __m128d lo = _mm256_castpd256_pd128(acc0);
    __m128d hi = _mm256_extractf128_pd(acc0, 1);
    lo = _mm_add_pd(lo, hi);
    sum = _mm_cvtsd_f64(lo) + _mm_cvtsd_f64(_mm_unpackhi_pd(lo, lo));
#endif

    /* scalar tail */
    for (; i < n; i++) {
        sum += a[i] * b[i];
    }
    *result = sum;
}

/* ------------------------------------------------------------------ */
/* simd_mat_vec_mul                                                    */
/* ------------------------------------------------------------------ */

void simd_mat_vec_mul(const double* mat, const double* vec, int rows, int cols, double* out) {
    int r;
    for (r = 0; r < rows; r++) {
        const double* row = mat + r * cols;
        double sum = 0.0;
        int j = 0;

#if USE_NEON
        float64x2_t acc0 = vdupq_n_f64(0.0);
        float64x2_t acc1 = vdupq_n_f64(0.0);
        float64x2_t acc2 = vdupq_n_f64(0.0);
        float64x2_t acc3 = vdupq_n_f64(0.0);
        for (; j + 7 < cols; j += 8) {
            acc0 = vfmaq_f64(acc0, vld1q_f64(row + j),     vld1q_f64(vec + j));
            acc1 = vfmaq_f64(acc1, vld1q_f64(row + j + 2), vld1q_f64(vec + j + 2));
            acc2 = vfmaq_f64(acc2, vld1q_f64(row + j + 4), vld1q_f64(vec + j + 4));
            acc3 = vfmaq_f64(acc3, vld1q_f64(row + j + 6), vld1q_f64(vec + j + 6));
        }
        acc0 = vaddq_f64(acc0, acc1);
        acc2 = vaddq_f64(acc2, acc3);
        acc0 = vaddq_f64(acc0, acc2);
        sum = vgetq_lane_f64(acc0, 0) + vgetq_lane_f64(acc0, 1);
#elif USE_AVX2
        __m256d acc0 = _mm256_setzero_pd();
        __m256d acc1 = _mm256_setzero_pd();
        __m256d acc2 = _mm256_setzero_pd();
        __m256d acc3 = _mm256_setzero_pd();
        for (; j + 15 < cols; j += 16) {
            acc0 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j),      _mm256_loadu_pd(vec + j),      acc0);
            acc1 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 4),  _mm256_loadu_pd(vec + j + 4),  acc1);
            acc2 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 8),  _mm256_loadu_pd(vec + j + 8),  acc2);
            acc3 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 12), _mm256_loadu_pd(vec + j + 12), acc3);
        }
        acc0 = _mm256_add_pd(acc0, acc1);
        acc2 = _mm256_add_pd(acc2, acc3);
        acc0 = _mm256_add_pd(acc0, acc2);
        __m128d lo = _mm256_castpd256_pd128(acc0);
        __m128d hi = _mm256_extractf128_pd(acc0, 1);
        lo = _mm_add_pd(lo, hi);
        sum = _mm_cvtsd_f64(lo) + _mm_cvtsd_f64(_mm_unpackhi_pd(lo, lo));
#endif

        for (; j < cols; j++) {
            sum += row[j] * vec[j];
        }
        out[r] = sum;
    }
}

/* ------------------------------------------------------------------ */
/* simd_vec_add                                                        */
/* ------------------------------------------------------------------ */

void simd_vec_add(const double* a, const double* b, int n, double* out) {
    int i = 0;

#if USE_NEON
    for (; i + 7 < n; i += 8) {
        vst1q_f64(out + i,     vaddq_f64(vld1q_f64(a + i),     vld1q_f64(b + i)));
        vst1q_f64(out + i + 2, vaddq_f64(vld1q_f64(a + i + 2), vld1q_f64(b + i + 2)));
        vst1q_f64(out + i + 4, vaddq_f64(vld1q_f64(a + i + 4), vld1q_f64(b + i + 4)));
        vst1q_f64(out + i + 6, vaddq_f64(vld1q_f64(a + i + 6), vld1q_f64(b + i + 6)));
    }
#elif USE_AVX2
    for (; i + 15 < n; i += 16) {
        _mm256_storeu_pd(out + i,      _mm256_add_pd(_mm256_loadu_pd(a + i),      _mm256_loadu_pd(b + i)));
        _mm256_storeu_pd(out + i + 4,  _mm256_add_pd(_mm256_loadu_pd(a + i + 4),  _mm256_loadu_pd(b + i + 4)));
        _mm256_storeu_pd(out + i + 8,  _mm256_add_pd(_mm256_loadu_pd(a + i + 8),  _mm256_loadu_pd(b + i + 8)));
        _mm256_storeu_pd(out + i + 12, _mm256_add_pd(_mm256_loadu_pd(a + i + 12), _mm256_loadu_pd(b + i + 12)));
    }
#endif

    for (; i < n; i++) {
        out[i] = a[i] + b[i];
    }
}

/* ------------------------------------------------------------------ */
/* simd_vec_mul                                                        */
/* ------------------------------------------------------------------ */

void simd_vec_mul(const double* a, const double* b, int n, double* out) {
    int i = 0;

#if USE_NEON
    for (; i + 7 < n; i += 8) {
        vst1q_f64(out + i,     vmulq_f64(vld1q_f64(a + i),     vld1q_f64(b + i)));
        vst1q_f64(out + i + 2, vmulq_f64(vld1q_f64(a + i + 2), vld1q_f64(b + i + 2)));
        vst1q_f64(out + i + 4, vmulq_f64(vld1q_f64(a + i + 4), vld1q_f64(b + i + 4)));
        vst1q_f64(out + i + 6, vmulq_f64(vld1q_f64(a + i + 6), vld1q_f64(b + i + 6)));
    }
#elif USE_AVX2
    for (; i + 15 < n; i += 16) {
        _mm256_storeu_pd(out + i,      _mm256_mul_pd(_mm256_loadu_pd(a + i),      _mm256_loadu_pd(b + i)));
        _mm256_storeu_pd(out + i + 4,  _mm256_mul_pd(_mm256_loadu_pd(a + i + 4),  _mm256_loadu_pd(b + i + 4)));
        _mm256_storeu_pd(out + i + 8,  _mm256_mul_pd(_mm256_loadu_pd(a + i + 8),  _mm256_loadu_pd(b + i + 8)));
        _mm256_storeu_pd(out + i + 12, _mm256_mul_pd(_mm256_loadu_pd(a + i + 12), _mm256_loadu_pd(b + i + 12)));
    }
#endif

    for (; i < n; i++) {
        out[i] = a[i] * b[i];
    }
}

/* ------------------------------------------------------------------ */
/* simd_rms_norm                                                       */
/* ------------------------------------------------------------------ */

void simd_rms_norm(const double* x, const double* weight, int n, double eps, double* out) {
    /* Pass 1: sum of squares */
    double ss = 0.0;
    int i = 0;

#if USE_NEON
    float64x2_t acc0 = vdupq_n_f64(0.0);
    float64x2_t acc1 = vdupq_n_f64(0.0);
    float64x2_t acc2 = vdupq_n_f64(0.0);
    float64x2_t acc3 = vdupq_n_f64(0.0);
    for (; i + 7 < n; i += 8) {
        float64x2_t v0 = vld1q_f64(x + i);
        float64x2_t v1 = vld1q_f64(x + i + 2);
        float64x2_t v2 = vld1q_f64(x + i + 4);
        float64x2_t v3 = vld1q_f64(x + i + 6);
        acc0 = vfmaq_f64(acc0, v0, v0);
        acc1 = vfmaq_f64(acc1, v1, v1);
        acc2 = vfmaq_f64(acc2, v2, v2);
        acc3 = vfmaq_f64(acc3, v3, v3);
    }
    acc0 = vaddq_f64(acc0, acc1);
    acc2 = vaddq_f64(acc2, acc3);
    acc0 = vaddq_f64(acc0, acc2);
    ss = vgetq_lane_f64(acc0, 0) + vgetq_lane_f64(acc0, 1);
#elif USE_AVX2
    __m256d a0 = _mm256_setzero_pd();
    __m256d a1 = _mm256_setzero_pd();
    for (; i + 7 < n; i += 8) {
        __m256d v0 = _mm256_loadu_pd(x + i);
        __m256d v1 = _mm256_loadu_pd(x + i + 4);
        a0 = _mm256_fmadd_pd(v0, v0, a0);
        a1 = _mm256_fmadd_pd(v1, v1, a1);
    }
    a0 = _mm256_add_pd(a0, a1);
    __m128d lo = _mm256_castpd256_pd128(a0);
    __m128d hi = _mm256_extractf128_pd(a0, 1);
    lo = _mm_add_pd(lo, hi);
    ss = _mm_cvtsd_f64(lo) + _mm_cvtsd_f64(_mm_unpackhi_pd(lo, lo));
#endif

    for (; i < n; i++) {
        ss += x[i] * x[i];
    }

    ss = ss / (double)n;
    double scale = 1.0 / sqrt(ss + eps);

    /* Pass 2: scale and multiply by weight */
    i = 0;

#if USE_NEON
    float64x2_t vs = vdupq_n_f64(scale);
    for (; i + 7 < n; i += 8) {
        vst1q_f64(out + i,     vmulq_f64(vmulq_f64(vld1q_f64(x + i),     vs), vld1q_f64(weight + i)));
        vst1q_f64(out + i + 2, vmulq_f64(vmulq_f64(vld1q_f64(x + i + 2), vs), vld1q_f64(weight + i + 2)));
        vst1q_f64(out + i + 4, vmulq_f64(vmulq_f64(vld1q_f64(x + i + 4), vs), vld1q_f64(weight + i + 4)));
        vst1q_f64(out + i + 6, vmulq_f64(vmulq_f64(vld1q_f64(x + i + 6), vs), vld1q_f64(weight + i + 6)));
    }
#elif USE_AVX2
    __m256d vscale = _mm256_set1_pd(scale);
    for (; i + 7 < n; i += 8) {
        _mm256_storeu_pd(out + i,     _mm256_mul_pd(_mm256_mul_pd(_mm256_loadu_pd(x + i),     vscale), _mm256_loadu_pd(weight + i)));
        _mm256_storeu_pd(out + i + 4, _mm256_mul_pd(_mm256_mul_pd(_mm256_loadu_pd(x + i + 4), vscale), _mm256_loadu_pd(weight + i + 4)));
    }
#endif

    for (; i < n; i++) {
        out[i] = x[i] * scale * weight[i];
    }
}

/* ------------------------------------------------------------------ */
/* simd_softmax                                                        */
/* ------------------------------------------------------------------ */

void simd_softmax(double* x, int n) {
    /* Find max for numerical stability */
    double maxVal = x[0];
    int i;
    for (i = 1; i < n; i++) {
        if (x[i] > maxVal) maxVal = x[i];
    }

    /* exp(x[i] - max) and sum */
    double sum = 0.0;
    for (i = 0; i < n; i++) {
        x[i] = exp(x[i] - maxVal);
        sum += x[i];
    }

    /* normalize */
    double inv_sum = 1.0 / sum;
    i = 0;

#if USE_NEON
    float64x2_t vs = vdupq_n_f64(inv_sum);
    for (; i + 7 < n; i += 8) {
        vst1q_f64(x + i,     vmulq_f64(vld1q_f64(x + i),     vs));
        vst1q_f64(x + i + 2, vmulq_f64(vld1q_f64(x + i + 2), vs));
        vst1q_f64(x + i + 4, vmulq_f64(vld1q_f64(x + i + 4), vs));
        vst1q_f64(x + i + 6, vmulq_f64(vld1q_f64(x + i + 6), vs));
    }
#elif USE_AVX2
    __m256d vs = _mm256_set1_pd(inv_sum);
    for (; i + 7 < n; i += 8) {
        _mm256_storeu_pd(x + i,     _mm256_mul_pd(_mm256_loadu_pd(x + i),     vs));
        _mm256_storeu_pd(x + i + 4, _mm256_mul_pd(_mm256_loadu_pd(x + i + 4), vs));
    }
#endif

    for (; i < n; i++) {
        x[i] *= inv_sum;
    }
}

/* ------------------------------------------------------------------ */
/* simd_silu_vec                                                       */
/* ------------------------------------------------------------------ */

void simd_silu_vec(double* x, int n) {
    /* SiLU(x) = x / (1 + exp(-x)) */
    int i;
    for (i = 0; i < n; i++) {
        x[i] = x[i] / (1.0 + exp(-x[i]));
    }
}

/* ------------------------------------------------------------------ */
/* simd_mat_vec_mul_off                                                */
/* ------------------------------------------------------------------ */

void simd_mat_vec_mul_off(const double* mat, int mat_off, const double* vec, int rows, int cols, double* out) {
    const double* base = mat + mat_off;
    int r;
    for (r = 0; r < rows; r++) {
        const double* row = base + r * cols;
        double sum = 0.0;
        int j = 0;

#if USE_NEON
        float64x2_t acc0 = vdupq_n_f64(0.0);
        float64x2_t acc1 = vdupq_n_f64(0.0);
        float64x2_t acc2 = vdupq_n_f64(0.0);
        float64x2_t acc3 = vdupq_n_f64(0.0);
        for (; j + 7 < cols; j += 8) {
            acc0 = vfmaq_f64(acc0, vld1q_f64(row + j),     vld1q_f64(vec + j));
            acc1 = vfmaq_f64(acc1, vld1q_f64(row + j + 2), vld1q_f64(vec + j + 2));
            acc2 = vfmaq_f64(acc2, vld1q_f64(row + j + 4), vld1q_f64(vec + j + 4));
            acc3 = vfmaq_f64(acc3, vld1q_f64(row + j + 6), vld1q_f64(vec + j + 6));
        }
        acc0 = vaddq_f64(acc0, acc1);
        acc2 = vaddq_f64(acc2, acc3);
        acc0 = vaddq_f64(acc0, acc2);
        sum = vgetq_lane_f64(acc0, 0) + vgetq_lane_f64(acc0, 1);
#elif USE_AVX2
        __m256d acc0 = _mm256_setzero_pd();
        __m256d acc1 = _mm256_setzero_pd();
        __m256d acc2 = _mm256_setzero_pd();
        __m256d acc3 = _mm256_setzero_pd();
        for (; j + 15 < cols; j += 16) {
            acc0 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j),      _mm256_loadu_pd(vec + j),      acc0);
            acc1 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 4),  _mm256_loadu_pd(vec + j + 4),  acc1);
            acc2 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 8),  _mm256_loadu_pd(vec + j + 8),  acc2);
            acc3 = _mm256_fmadd_pd(_mm256_loadu_pd(row + j + 12), _mm256_loadu_pd(vec + j + 12), acc3);
        }
        acc0 = _mm256_add_pd(acc0, acc1);
        acc2 = _mm256_add_pd(acc2, acc3);
        acc0 = _mm256_add_pd(acc0, acc2);
        __m128d lo = _mm256_castpd256_pd128(acc0);
        __m128d hi = _mm256_extractf128_pd(acc0, 1);
        lo = _mm_add_pd(lo, hi);
        sum = _mm_cvtsd_f64(lo) + _mm_cvtsd_f64(_mm_unpackhi_pd(lo, lo));
#endif

        for (; j < cols; j++) {
            sum += row[j] * vec[j];
        }
        out[r] = sum;
    }
}

/* ------------------------------------------------------------------ */
/* simd_vec_dot_off                                                    */
/* ------------------------------------------------------------------ */

void simd_vec_dot_off(const double* a, int a_off, const double* b, int b_off, int n, double* result) {
    const double* ap = a + a_off;
    const double* bp = b + b_off;
    double sum = 0.0;
    int i = 0;

#if USE_NEON
    float64x2_t acc0 = vdupq_n_f64(0.0);
    float64x2_t acc1 = vdupq_n_f64(0.0);
    float64x2_t acc2 = vdupq_n_f64(0.0);
    float64x2_t acc3 = vdupq_n_f64(0.0);
    for (; i + 7 < n; i += 8) {
        acc0 = vfmaq_f64(acc0, vld1q_f64(ap + i),     vld1q_f64(bp + i));
        acc1 = vfmaq_f64(acc1, vld1q_f64(ap + i + 2), vld1q_f64(bp + i + 2));
        acc2 = vfmaq_f64(acc2, vld1q_f64(ap + i + 4), vld1q_f64(bp + i + 4));
        acc3 = vfmaq_f64(acc3, vld1q_f64(ap + i + 6), vld1q_f64(bp + i + 6));
    }
    acc0 = vaddq_f64(acc0, acc1);
    acc2 = vaddq_f64(acc2, acc3);
    acc0 = vaddq_f64(acc0, acc2);
    sum = vgetq_lane_f64(acc0, 0) + vgetq_lane_f64(acc0, 1);
#elif USE_AVX2
    __m256d acc0 = _mm256_setzero_pd();
    __m256d acc1 = _mm256_setzero_pd();
    __m256d acc2 = _mm256_setzero_pd();
    __m256d acc3 = _mm256_setzero_pd();
    for (; i + 15 < n; i += 16) {
        acc0 = _mm256_fmadd_pd(_mm256_loadu_pd(ap + i),      _mm256_loadu_pd(bp + i),      acc0);
        acc1 = _mm256_fmadd_pd(_mm256_loadu_pd(ap + i + 4),  _mm256_loadu_pd(bp + i + 4),  acc1);
        acc2 = _mm256_fmadd_pd(_mm256_loadu_pd(ap + i + 8),  _mm256_loadu_pd(bp + i + 8),  acc2);
        acc3 = _mm256_fmadd_pd(_mm256_loadu_pd(ap + i + 12), _mm256_loadu_pd(bp + i + 12), acc3);
    }
    acc0 = _mm256_add_pd(acc0, acc1);
    acc2 = _mm256_add_pd(acc2, acc3);
    acc0 = _mm256_add_pd(acc0, acc2);
    __m128d lo = _mm256_castpd256_pd128(acc0);
    __m128d hi = _mm256_extractf128_pd(acc0, 1);
    lo = _mm_add_pd(lo, hi);
    sum = _mm_cvtsd_f64(lo) + _mm_cvtsd_f64(_mm_unpackhi_pd(lo, lo));
#endif

    /* scalar tail */
    for (; i < n; i++) {
        sum += ap[i] * bp[i];
    }
    *result = sum;
}
