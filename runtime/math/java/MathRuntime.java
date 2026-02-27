// MathRuntime.java - Math runtime for goany math package
// Uses optimized scalar loops with ArrayList<Double>

import java.util.*;

class math {

    public static double VecDot(ArrayList<Double> a, ArrayList<Double> b, long n) {
        double sum = 0.0;
        int len = (int) n;
        int i = 0;
        for (; i + 3 < len; i += 4) {
            sum += a.get(i) * b.get(i) + a.get(i+1) * b.get(i+1) + a.get(i+2) * b.get(i+2) + a.get(i+3) * b.get(i+3);
        }
        for (; i < len; i++) {
            sum += a.get(i) * b.get(i);
        }
        return sum;
    }

    public static ArrayList<Double> MatVecMul(ArrayList<Double> mat, ArrayList<Double> vec, long rows, long cols) {
        int r = (int) rows;
        int c = (int) cols;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < r; i++) out.add(0.0);
        for (int i = 0; i < r; i++) {
            double sum = 0.0;
            int off = i * c;
            int j = 0;
            for (; j + 3 < c; j += 4) {
                sum += mat.get(off+j) * vec.get(j) + mat.get(off+j+1) * vec.get(j+1) + mat.get(off+j+2) * vec.get(j+2) + mat.get(off+j+3) * vec.get(j+3);
            }
            for (; j < c; j++) {
                sum += mat.get(off+j) * vec.get(j);
            }
            out.set(i, sum);
        }
        return out;
    }

    public static ArrayList<Double> VecAdd(ArrayList<Double> a, ArrayList<Double> b, long n) {
        int len = (int) n;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < len; i++) out.add(0.0);
        for (int i = 0; i < len; i++) {
            out.set(i, a.get(i) + b.get(i));
        }
        return out;
    }

    public static ArrayList<Double> VecMul(ArrayList<Double> a, ArrayList<Double> b, long n) {
        int len = (int) n;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < len; i++) out.add(0.0);
        for (int i = 0; i < len; i++) {
            out.set(i, a.get(i) * b.get(i));
        }
        return out;
    }

    public static ArrayList<Double> RMSNorm(ArrayList<Double> x, ArrayList<Double> weight, long n, double eps) {
        int len = (int) n;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < len; i++) out.add(0.0);
        double ss = 0.0;
        for (int i = 0; i < len; i++) {
            ss += x.get(i) * x.get(i);
        }
        ss = ss / (double) len;
        double scale = 1.0 / Math.sqrt(ss + eps);
        for (int i = 0; i < len; i++) {
            out.set(i, x.get(i) * scale * weight.get(i));
        }
        return out;
    }

    public static ArrayList<Double> Softmax(ArrayList<Double> x, long n) {
        int len = (int) n;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < len; i++) out.add(x.get(i));
        double maxVal = out.get(0);
        for (int i = 1; i < len; i++) {
            if (out.get(i) > maxVal) maxVal = out.get(i);
        }
        double sum = 0.0;
        for (int i = 0; i < len; i++) {
            out.set(i, Math.exp(out.get(i) - maxVal));
            sum += out.get(i);
        }
        double invSum = 1.0 / sum;
        for (int i = 0; i < len; i++) {
            out.set(i, out.get(i) * invSum);
        }
        return out;
    }

    public static ArrayList<Double> SiLUVec(ArrayList<Double> x, long n) {
        int len = (int) n;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < len; i++) out.add(0.0);
        for (int i = 0; i < len; i++) {
            out.set(i, x.get(i) / (1.0 + Math.exp(-x.get(i))));
        }
        return out;
    }

    public static ArrayList<Double> MatVecMulOff(ArrayList<Double> mat, long matOff, ArrayList<Double> vec, long rows, long cols) {
        int r = (int) rows;
        int c = (int) cols;
        int mOff = (int) matOff;
        ArrayList<Double> out = new ArrayList<Double>();
        for (int i = 0; i < r; i++) out.add(0.0);
        for (int i = 0; i < r; i++) {
            double sum = 0.0;
            int off = mOff + i * c;
            int j = 0;
            for (; j + 3 < c; j += 4) {
                sum += mat.get(off+j) * vec.get(j) + mat.get(off+j+1) * vec.get(j+1) + mat.get(off+j+2) * vec.get(j+2) + mat.get(off+j+3) * vec.get(j+3);
            }
            for (; j < c; j++) {
                sum += mat.get(off+j) * vec.get(j);
            }
            out.set(i, sum);
        }
        return out;
    }

    public static double VecDotOff(ArrayList<Double> a, long aOff, ArrayList<Double> b, long bOff, long n) {
        double sum = 0.0;
        int len = (int) n;
        int ao = (int) aOff;
        int bo = (int) bOff;
        int i = 0;
        for (; i + 3 < len; i += 4) {
            sum += a.get(ao+i) * b.get(bo+i) + a.get(ao+i+1) * b.get(bo+i+1) + a.get(ao+i+2) * b.get(bo+i+2) + a.get(ao+i+3) * b.get(bo+i+3);
        }
        for (; i < len; i++) {
            sum += a.get(ao+i) * b.get(bo+i);
        }
        return sum;
    }
}
