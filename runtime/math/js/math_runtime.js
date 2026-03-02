// math_runtime.js - Scalar math runtime for JavaScript
// JS has no float64 SIMD, so this uses optimized scalar loops

const math = {
    VecDot: function(a, b, n) {
        let sum = 0.0;
        let i = 0;
        for (; i + 3 < n; i += 4) {
            sum += a[i] * b[i] + a[i+1] * b[i+1] + a[i+2] * b[i+2] + a[i+3] * b[i+3];
        }
        for (; i < n; i++) {
            sum += a[i] * b[i];
        }
        return sum;
    },

    MatVecMul: function(mat, vec, rows, cols) {
        let out = new Array(rows).fill(0.0);
        for (let r = 0; r < rows; r++) {
            let sum = 0.0;
            let off = r * cols;
            let j = 0;
            for (; j + 3 < cols; j += 4) {
                sum += mat[off+j] * vec[j] + mat[off+j+1] * vec[j+1] + mat[off+j+2] * vec[j+2] + mat[off+j+3] * vec[j+3];
            }
            for (; j < cols; j++) {
                sum += mat[off+j] * vec[j];
            }
            out[r] = sum;
        }
        return out;
    },

    VecAdd: function(a, b, n) {
        let out = new Array(n).fill(0.0);
        for (let i = 0; i < n; i++) {
            out[i] = a[i] + b[i];
        }
        return out;
    },

    VecMul: function(a, b, n) {
        let out = new Array(n).fill(0.0);
        for (let i = 0; i < n; i++) {
            out[i] = a[i] * b[i];
        }
        return out;
    },

    RMSNorm: function(x, weight, n, eps) {
        let out = new Array(n).fill(0.0);
        let ss = 0.0;
        for (let i = 0; i < n; i++) {
            ss += x[i] * x[i];
        }
        ss = ss / n;
        let scale = 1.0 / Math.sqrt(ss + eps);
        for (let i = 0; i < n; i++) {
            out[i] = x[i] * scale * weight[i];
        }
        return out;
    },

    Softmax: function(x, n) {
        let out = x.slice(0, n);
        let maxVal = out[0];
        for (let i = 1; i < n; i++) {
            if (out[i] > maxVal) maxVal = out[i];
        }
        let sum = 0.0;
        for (let i = 0; i < n; i++) {
            out[i] = Math.exp(out[i] - maxVal);
            sum += out[i];
        }
        let invSum = 1.0 / sum;
        for (let i = 0; i < n; i++) {
            out[i] *= invSum;
        }
        return out;
    },

    SiLUVec: function(x, n) {
        let out = new Array(n).fill(0.0);
        for (let i = 0; i < n; i++) {
            out[i] = x[i] / (1.0 + Math.exp(-x[i]));
        }
        return out;
    },

    MatVecMulOff: function(mat, matOff, vec, rows, cols) {
        let out = new Array(rows).fill(0.0);
        for (let r = 0; r < rows; r++) {
            let sum = 0.0;
            let off = matOff + r * cols;
            let j = 0;
            for (; j + 3 < cols; j += 4) {
                sum += mat[off+j] * vec[j] + mat[off+j+1] * vec[j+1] + mat[off+j+2] * vec[j+2] + mat[off+j+3] * vec[j+3];
            }
            for (; j < cols; j++) {
                sum += mat[off+j] * vec[j];
            }
            out[r] = sum;
        }
        return out;
    },

    VecDotOff: function(a, aOff, b, bOff, n) {
        let sum = 0.0;
        let i = 0;
        for (; i + 3 < n; i += 4) {
            sum += a[aOff+i] * b[bOff+i] + a[aOff+i+1] * b[bOff+i+1] + a[aOff+i+2] * b[bOff+i+2] + a[aOff+i+3] * b[bOff+i+3];
        }
        for (; i < n; i++) {
            sum += a[aOff+i] * b[bOff+i];
        }
        return sum;
    }
};
