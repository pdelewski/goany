// math_runtime.rs - SIMD math runtime for goany math package
// Uses FFI to math_simd.c (compiled via build.rs)

use std::os::raw::{c_double, c_int};

extern "C" {
    fn simd_vec_dot(a: *const c_double, b: *const c_double, n: c_int, result: *mut c_double);
    fn simd_mat_vec_mul(mat: *const c_double, vec: *const c_double, rows: c_int, cols: c_int, out: *mut c_double);
    fn simd_vec_add(a: *const c_double, b: *const c_double, n: c_int, out: *mut c_double);
    fn simd_vec_mul(a: *const c_double, b: *const c_double, n: c_int, out: *mut c_double);
    fn simd_rms_norm(x: *const c_double, weight: *const c_double, n: c_int, eps: c_double, out: *mut c_double);
    fn simd_softmax(x: *mut c_double, n: c_int);
    fn simd_silu_vec(x: *mut c_double, n: c_int);
    fn simd_mat_vec_mul_off(mat: *const c_double, mat_off: c_int, vec: *const c_double, rows: c_int, cols: c_int, out: *mut c_double);
    fn simd_vec_dot_off(a: *const c_double, a_off: c_int, b: *const c_double, b_off: c_int, n: c_int, result: *mut c_double);
}

pub fn VecDot(a: Vec<f64>, b: Vec<f64>, n: i32) -> f64 {
    let mut result: f64 = 0.0;
    unsafe {
        simd_vec_dot(a.as_ptr(), b.as_ptr(), n as c_int, &mut result);
    }
    result
}

pub fn MatVecMul(mat: Vec<f64>, vec_arg: Vec<f64>, rows: i32, cols: i32) -> Vec<f64> {
    let mut out = vec![0.0f64; rows as usize];
    unsafe {
        simd_mat_vec_mul(mat.as_ptr(), vec_arg.as_ptr(), rows as c_int, cols as c_int, out.as_mut_ptr());
    }
    out
}

pub fn VecAdd(a: Vec<f64>, b: Vec<f64>, n: i32) -> Vec<f64> {
    let mut out = vec![0.0f64; n as usize];
    unsafe {
        simd_vec_add(a.as_ptr(), b.as_ptr(), n as c_int, out.as_mut_ptr());
    }
    out
}

pub fn VecMul(a: Vec<f64>, b: Vec<f64>, n: i32) -> Vec<f64> {
    let mut out = vec![0.0f64; n as usize];
    unsafe {
        simd_vec_mul(a.as_ptr(), b.as_ptr(), n as c_int, out.as_mut_ptr());
    }
    out
}

pub fn RMSNorm(x: Vec<f64>, weight: Vec<f64>, n: i32, eps: f64) -> Vec<f64> {
    let mut out = vec![0.0f64; n as usize];
    unsafe {
        simd_rms_norm(x.as_ptr(), weight.as_ptr(), n as c_int, eps, out.as_mut_ptr());
    }
    out
}

pub fn Softmax(x: Vec<f64>, n: i32) -> Vec<f64> {
    let mut out = x[..n as usize].to_vec();
    unsafe {
        simd_softmax(out.as_mut_ptr(), n as c_int);
    }
    out
}

pub fn SiLUVec(x: Vec<f64>, n: i32) -> Vec<f64> {
    let mut out = x[..n as usize].to_vec();
    unsafe {
        simd_silu_vec(out.as_mut_ptr(), n as c_int);
    }
    out
}

pub fn MatVecMulOff(mat: Vec<f64>, mat_off: i32, vec_arg: Vec<f64>, rows: i32, cols: i32) -> Vec<f64> {
    let mut out = vec![0.0f64; rows as usize];
    unsafe {
        simd_mat_vec_mul_off(mat.as_ptr(), mat_off as c_int, vec_arg.as_ptr(), rows as c_int, cols as c_int, out.as_mut_ptr());
    }
    out
}

pub fn VecDotOff(a: Vec<f64>, a_off: i32, b: Vec<f64>, b_off: i32, n: i32) -> f64 {
    let mut result: f64 = 0.0;
    unsafe {
        simd_vec_dot_off(a.as_ptr(), a_off as c_int, b.as_ptr(), b_off as c_int, n as c_int, &mut result);
    }
    result
}
