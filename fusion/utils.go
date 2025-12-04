package fusion

import (
    "math"
    "gonum.org/v1/gonum/mat"
)

// Anchor describes a beacon/anchor position and metadata.
type Anchor struct {
    ID       int
    X, Y, Z  float64
    Layer    int
    Building int
}

// BLERow mirrors one BLE measurement row (x,y,z,strength,anchorID,layer,reserved).
type BLERow struct {
    X, Y, Z float64
    Strength float64
    AnchorID int
    Layer    int
    Reserved float64
}

// TWRRow mirrors one UWB TWR measurement row (x,y,z,range_m,anchorID,layer,reserved,B_h).
type TWRRow struct {
    X, Y, Z float64
    Range   float64
    AnchorID int
    Layer    int
    Reserved float64
    Bh       float64
}

// DimMat represents a dimension constraint matrix of shape (n,3).
type DimMat [][]float64

// RandomModel emulates Function_Utils::Random_Model from C++.
func RandomModel(x float64, modelType string) float64 {
    switch modelType {
    case "dd":
        if x <= 3 {
            return 5.0 * (math.Pow(2, 2*x-4.5) + 0.2)
        }
        return 5.0 * (-math.Pow(2, -x+5.58) + 9.0)
    case "dh":
        if x <= 0 || x > 20 {
            return 0.5
        } else if x > 2 && x <= 6 {
            return 0.9
        } else if x > 6 {
            return 0.7
        }
        return 1.0
    case "ble":
        if x <= 15 {
            return (math.Pow(2, 0.45*x-5.3) + 0.2) / 3.0
        } else if x <= 40 {
            return (-math.Pow(2, -0.2*x+5.34) + 8.0) / 3.0
        }
        return 3.3
    case "tof":
        if x < 1e-1 {
            return 100.0
        } else if x < 10 {
            return 0.9
        } else if x < 30 {
            return 2.0
        } else if x < 50 {
            return 5.0
        }
        return 10.0
    case "MH":
        if x <= 0 || x > 20 {
            return 2.0
        } else if x > 6 {
            return 1.5
        } else if x > 3 {
            return 1.1
        }
        return 1.0
    default:
        return 1.0
    }
}

// Chi2Inv returns approximate inverse chi-square for df (1..10) at p=0.99 or 0.95.
func Chi2Inv(p float64, df int) float64 {
    table := chi2_05
    if p >= 0.97 {
        table = chi2_01
    }
    if df < 1 {
        return table[0]
    }
    if df > 10 {
        return table[len(table)-1]
    }
    return table[df-1]
}

// RKStatistics computes statistics of innovations (mean, std, chi-square).
func RKStatistics(meaSize int, rk []float64, pykk1 [][]float64) [3]float64 {
    diagS := make([]float64, meaSize)
    for i := 0; i < meaSize; i++ {
        v := pykk1[i][i]
        if v < 1e-9 {
            v = 1e-9
        }
        diagS[i] = v
    }
    stand := make([]float64, meaSize)
    nisEach := make([]float64, meaSize)
    sum := 0.0
    for i := 0; i < meaSize; i++ {
        stand[i] = rk[i] / math.Sqrt(diagS[i])
        nisEach[i] = stand[i] * stand[i]
        sum += rk[i]
    }
    mean := sum / float64(meaSize)
    varVar := 0.0
    for i := 0; i < meaSize; i++ {
        d := rk[i] - mean
        varVar += d * d
    }
    stddev := math.Sqrt(varVar / math.Max(float64(meaSize-1), 1.0))

    // chi-square = rk^T * pinv(Pykk_1) * rk; approximate using naive inverse
    inv := pinv(pykk1)
    chi := 0.0
    for i := 0; i < meaSize; i++ {
        for j := 0; j < meaSize; j++ {
            chi += rk[i] * inv[i][j] * rk[j]
        }
    }
    return [3]float64{mean, stddev, chi}
}

// pinv computes pseudo-inverse via SVD (using Gonum).
func pinv(a [][]float64) [][]float64 {
    r := len(a)
    if r == 0 {
        return [][]float64{}
    }
    c := len(a[0])

    // Flatten input to construct Gonum matrix
    data := make([]float64, 0, r*c)
    for _, row := range a {
        data = append(data, row...)
    }
    A := mat.NewDense(r, c, data)

    // Compute SVD
    var svd mat.SVD
    ok := svd.Factorize(A, mat.SVDThin)
    if !ok {
        // Should not happen for valid inputs, return zero matrix on failure
        m := make([][]float64, c)
        for i := range m {
            m[i] = make([]float64, r)
        }
        return m
    }

    var u, v mat.Dense
    svd.UTo(&u)
    svd.VTo(&v)
    s := svd.Values(nil)

    // Construct Sigma pseudoinverse
    // Threshold for singular values
    maxS := 0.0
    if len(s) > 0 {
        maxS = s[0]
    }
    // Standard tolerance: eps * max(rows, cols) * max_singular_value
    tol := 1e-15 * float64(max(r, c)) * maxS

    sigInv := mat.NewDense(len(s), len(s), nil)
    for i, val := range s {
        if val > tol {
            sigInv.Set(i, i, 1.0/val)
        }
    }

    // Pseudo-inverse = V * Sigma^+ * U^T
    var temp mat.Dense
    temp.Mul(&v, sigInv)
    var res mat.Dense
    res.Mul(&temp, u.T())

    // Convert back to [][]float64
    rows, cols := res.Dims()
    out := make([][]float64, rows)
    for i := 0; i < rows; i++ {
        out[i] = make([]float64, cols)
        copy(out[i], res.RawRowView(i))
    }
    return out
}

