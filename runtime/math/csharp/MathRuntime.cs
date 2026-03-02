// MathRuntime.cs - Math runtime for goany math package
// Uses scalar loops with List<double> (C# JIT auto-vectorizes)

using System;
using System.Collections.Generic;

public static class math
{
    public static double VecDot(List<double> a, List<double> b, long n)
    {
        double sum = 0.0;
        int len = (int)n;
        int i = 0;
        for (; i + 3 < len; i += 4)
        {
            sum += a[i] * b[i] + a[i+1] * b[i+1] + a[i+2] * b[i+2] + a[i+3] * b[i+3];
        }
        for (; i < len; i++)
        {
            sum += a[i] * b[i];
        }
        return sum;
    }

    public static List<double> MatVecMul(List<double> mat, List<double> vec, long rows, long cols)
    {
        int r = (int)rows;
        int c = (int)cols;
        List<double> outList = new List<double>(r);
        for (int i = 0; i < r; i++) outList.Add(0.0);
        for (int i = 0; i < r; i++)
        {
            double sum = 0.0;
            int off = i * c;
            int j = 0;
            for (; j + 3 < c; j += 4)
            {
                sum += mat[off+j] * vec[j] + mat[off+j+1] * vec[j+1] + mat[off+j+2] * vec[j+2] + mat[off+j+3] * vec[j+3];
            }
            for (; j < c; j++)
            {
                sum += mat[off+j] * vec[j];
            }
            outList[i] = sum;
        }
        return outList;
    }

    public static List<double> VecAdd(List<double> a, List<double> b, long n)
    {
        int len = (int)n;
        List<double> outList = new List<double>(len);
        for (int i = 0; i < len; i++) outList.Add(0.0);
        for (int i = 0; i < len; i++)
        {
            outList[i] = a[i] + b[i];
        }
        return outList;
    }

    public static List<double> VecMul(List<double> a, List<double> b, long n)
    {
        int len = (int)n;
        List<double> outList = new List<double>(len);
        for (int i = 0; i < len; i++) outList.Add(0.0);
        for (int i = 0; i < len; i++)
        {
            outList[i] = a[i] * b[i];
        }
        return outList;
    }

    public static List<double> RMSNorm(List<double> x, List<double> weight, long n, double eps)
    {
        int len = (int)n;
        List<double> outList = new List<double>(len);
        for (int i = 0; i < len; i++) outList.Add(0.0);
        double ss = 0.0;
        for (int i = 0; i < len; i++)
        {
            ss += x[i] * x[i];
        }
        ss = ss / (double)len;
        double scale = 1.0 / Math.Sqrt(ss + eps);
        for (int i = 0; i < len; i++)
        {
            outList[i] = x[i] * scale * weight[i];
        }
        return outList;
    }

    public static List<double> Softmax(List<double> x, long n)
    {
        int len = (int)n;
        List<double> outList = new List<double>(len);
        for (int i = 0; i < len; i++) outList.Add(x[i]);
        double maxVal = outList[0];
        for (int i = 1; i < len; i++)
        {
            if (outList[i] > maxVal) maxVal = outList[i];
        }
        double sum = 0.0;
        for (int i = 0; i < len; i++)
        {
            outList[i] = Math.Exp(outList[i] - maxVal);
            sum += outList[i];
        }
        double invSum = 1.0 / sum;
        for (int i = 0; i < len; i++)
        {
            outList[i] *= invSum;
        }
        return outList;
    }

    public static List<double> SiLUVec(List<double> x, long n)
    {
        int len = (int)n;
        List<double> outList = new List<double>(len);
        for (int i = 0; i < len; i++) outList.Add(0.0);
        for (int i = 0; i < len; i++)
        {
            outList[i] = x[i] / (1.0 + Math.Exp(-x[i]));
        }
        return outList;
    }

    public static List<double> MatVecMulOff(List<double> mat, long matOff, List<double> vec, long rows, long cols)
    {
        int r = (int)rows;
        int c = (int)cols;
        int mOff = (int)matOff;
        List<double> outList = new List<double>(r);
        for (int i = 0; i < r; i++) outList.Add(0.0);
        for (int i = 0; i < r; i++)
        {
            double sum = 0.0;
            int off = mOff + i * c;
            int j = 0;
            for (; j + 3 < c; j += 4)
            {
                sum += mat[off+j] * vec[j] + mat[off+j+1] * vec[j+1] + mat[off+j+2] * vec[j+2] + mat[off+j+3] * vec[j+3];
            }
            for (; j < c; j++)
            {
                sum += mat[off+j] * vec[j];
            }
            outList[i] = sum;
        }
        return outList;
    }

    public static double VecDotOff(List<double> a, long aOff, List<double> b, long bOff, long n)
    {
        double sum = 0.0;
        int len = (int)n;
        int ao = (int)aOff;
        int bo = (int)bOff;
        int i = 0;
        for (; i + 3 < len; i += 4)
        {
            sum += a[ao+i] * b[bo+i] + a[ao+i+1] * b[bo+i+1] + a[ao+i+2] * b[bo+i+2] + a[ao+i+3] * b[bo+i+3];
        }
        for (; i < len; i++)
        {
            sum += a[ao+i] * b[bo+i];
        }
        return sum;
    }
}
