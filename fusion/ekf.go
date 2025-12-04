package fusion

import (
    "math"
)

type EKFSample struct {
    Timestamp int64
    TagID     int
    TagHeight float64
    BLE       []BLERow
    TWR       []TWRRow
    DimPos    []DimMat
}

type EKF struct {
    n        int
    m        int
    ts       float64
    fading   float64
    adaptive bool
    beta     float64
    b        float64

    xconstrain []bool
    xMin       []float64
    xMax       []float64

    usedMea []int
    ret     int
    HDOP    float64
    HMaha   float64

    BLE2Dis [][]float64

    xk    []float64
    Pxk   [][]float64
    Phikk1 [][]float64
    Qk    [][]float64

    yk   []float64
    ykk1 []float64
    Hk   [][]float64
    Rk   [][]float64
    Rmin [][]float64
    Rmax [][]float64

    Dc    *DimConstrain
    xkk1  []float64
    Pykk1 [][]float64
    rk    []float64
}

func NewEKF() *EKF {
    k := &EKF{}
    k.n = StateDim
    k.m = MaxMeaDim
    k.ts = 0.1
    k.fading = Fading
    k.adaptive = UseAdaptive
    k.beta = BetaInit
    k.b = BetaB
    k.xconstrain = []bool{false, false, true, true, true, true}
    k.xMin = make([]float64, k.n)
    k.xMax = make([]float64, k.n)
    k.xMin[2] = -MaxVel
    k.xMin[3] = -MaxVel
    k.xMin[4] = PathLossExp[0]
    k.xMin[5] = DeltaA[0]
    k.xMax[2] = MaxVel
    k.xMax[3] = MaxVel
    k.xMax[4] = PathLossExp[2]
    k.xMax[5] = DeltaA[2]
    k.usedMea = make([]int, 4)
    k.BLE2Dis = make([][]float64, k.m)
    for i := 0; i < k.m; i++ {
        k.BLE2Dis[i] = make([]float64, 3)
    }
    k.Dc = NewDimConstrain(HistoryLen)
    k.xkk1 = make([]float64, k.n)
    k.resetState()
    return k
}

func (k *EKF) resetState() {
    k.xk = make([]float64, k.n)
    k.xk[4] = PathLossExp[1]
    k.xk[5] = DeltaA[1]
    k.Pxk = zeroMat(k.n, k.n)
    k.Pxk[0][0] = Pow2(SigmaPos)
    k.Pxk[1][1] = Pow2(SigmaPos)
    k.Pxk[2][2] = Pow2(SigmaVel)
    k.Pxk[3][3] = Pow2(SigmaVel)
    k.Pxk[4][4] = Pow2(SigmaN0)
    k.Pxk[5][5] = Pow2(SigmaA0)
    k.Phikk1 = identity(k.n)
    k.Qk = zeroMat(k.n, k.n)
}

func (k *EKF) Updt(dtime float64) {
    k.ts = dtime
    k.Phikk1 = identity(k.n)
    k.Phikk1[0][2] = dtime
    k.Phikk1[1][3] = dtime
    qx := Pow2(SigmaAcc)
    qn := Pow2(SigmaN)
    qA := Pow2(SigmaA)
    k.Qk = zeroMat(k.n, k.n)
    k.Qk[0][0] = (math.Pow(dtime, 3) / 3.0) * qx
    k.Qk[0][2] = (math.Pow(dtime, 2) / 2.0) * qx
    k.Qk[2][0] = k.Qk[0][2]
    k.Qk[2][2] = dtime * qx
    k.Qk[1][1] = (math.Pow(dtime, 3) / 3.0) * qx
    k.Qk[1][3] = (math.Pow(dtime, 2) / 2.0) * qx
    k.Qk[3][1] = k.Qk[1][3]
    k.Qk[3][3] = dtime * qx
    nAScale := 1.0
    if k.usedMea[1] == 0 {
        nAScale = 0.01 * 0.01
    }
    k.Qk[4][4] = dtime * qn * nAScale
    k.Qk[5][5] = dtime * qA * nAScale
}

func (k *EKF) UpMeas(sample *EKFSample) {
    k.usedMea[0] = len(sample.TWR)
    k.usedMea[1] = len(sample.BLE)
    k.usedMea[2] = 0
    k.usedMea[3] = 0
    k.Dc.DimConsDeter(sample, k)
    total := k.usedMea[0] + k.usedMea[1] + k.usedMea[3]
    k.yk = make([]float64, total)
    k.ykk1 = make([]float64, total)
    k.Hk = zeroMat(total, k.n)
    k.Rk = zeroMat(total, total)
    k.Rmin = zeroMat(total, total)
    k.Rmax = zeroMat(total, total)

    idx := 0
    for _, tw := range sample.TWR {
        k.yk[idx] = tw.Range
        idx++
    }
    for _, bl := range sample.BLE {
        k.yk[idx] = bl.Strength
        idx++
    }
    for _, use := range k.Dc.dimConsUse {
        if use {
            k.yk[idx] = 0.0
            idx++
        }
    }

    idx = 0
    // Hk for TWR
    for _, tw := range sample.TWR {
        dx := k.xk[0] - tw.X
        dy := k.xk[1] - tw.Y
        dz := sample.TagHeight - tw.Z
        d := math.Hypot(dx, dy)
        d = math.Sqrt(d*d + dz*dz)
        if d < MinDistance {
            d = MinDistance
        }
        k.Hk[idx][0] = dx / d
        k.Hk[idx][1] = dy / d
        idx++
    }
    // Hk for BLE
    for _, bl := range sample.BLE {
        dx := k.xk[0] - bl.X
        dy := k.xk[1] - bl.Y
        dz := sample.TagHeight - bl.Z
        d := math.Hypot(dx, dy)
        d = math.Sqrt(d*d + dz*dz)
        if d < MinDistance {
            d = MinDistance
        }
        common := 10.0 * k.xk[4] / (Ln10 * d * d)
        k.Hk[idx][0] = common * dx
        k.Hk[idx][1] = common * dy
        k.Hk[idx][4] = 10.0 * math.Log10(d)
        k.Hk[idx][5] = 1.0
        idx++
    }

    // HDOP
    totalMea := k.usedMea[0] + k.usedMea[1]
    if totalMea >= 2 {
        hxy := make([][]float64, totalMea)
        for i := 0; i < totalMea; i++ {
            hxy[i] = []float64{k.Hk[i][0], k.Hk[i][1]}
        }
        g := matMul(transpose(hxy), hxy)
        if rank2(g) == 2 {
            ginv := invert2x2(g)
            k.HDOP = math.Sqrt(ginv[0][0] + ginv[1][1])
            if k.HDOP > HDOPMax {
                k.HDOP = HDOPMax
            }
        } else {
            k.HDOP = 0.0
        }
    } else {
        k.HDOP = 0.0
    }

    idx = 0
    fHdop := RandomModel(k.HDOP, "MH")
    for _, tw := range sample.TWR {
        fDis := RandomModel(tw.Range, "tof")
        k.Rk[idx][idx] = Pow2(ToFErr * fDis * fHdop)
        idx++
    }
    for _, bl := range sample.BLE {
        fRssi := RandomModel(bl.Strength, "ble")
        k.Rk[idx][idx] = Pow2(BleErr * fRssi * fHdop)
        idx++
    }
    // dim noises set in ConsHk (later)
    for i := 0; i < total; i++ {
        k.Rmax[i][i] = 100.0 * k.Rk[i][i]
        k.Rmin[i][i] = 0.01 * k.Rk[i][i]
    }
    k.ManagePxk()
}

func (k *EKF) KfUpdate(sample *EKFSample) {
    total := k.usedMea[0] + k.usedMea[1] + k.usedMea[3]
    if total == 0 {
        // predict only
        k.xk = matVec(k.Phikk1, k.xk)
        k.Pxk = matAdd(matMul(k.Phikk1, matMul(k.Pxk, transpose(k.Phikk1))), k.Qk)
        k.ret = 1
        return
    }

    k.xkk1 = matVec(k.Phikk1, k.xk)
    Pxkk1 := matAdd(matMul(k.Phikk1, matMul(k.Pxk, transpose(k.Phikk1))), k.Qk)

    // dimension constraint uses predicted state
    k.Dc.ConsHk(sample, k)

    // hx pre
    idx := 0
    o := 0
    for _, tw := range sample.TWR {
        dx := k.xkk1[0] - tw.X
        dy := k.xkk1[1] - tw.Y
        dz := sample.TagHeight - tw.Z
        d := math.Hypot(dx, dy)
        d = math.Sqrt(d*d + dz*dz)
        if d < MinDistance {
            d = MinDistance
        }
        k.ykk1[idx] = d
        idx++
    }
    for _, bl := range sample.BLE {
        dx := k.xkk1[0] - bl.X
        dy := k.xkk1[1] - bl.Y
        dz := sample.TagHeight - bl.Z
        d := math.Hypot(dx, dy)
        d = math.Sqrt(d*d + dz*dz)
        if d < MinDistance {
            d = MinDistance
        }
        k.ykk1[idx] = k.xkk1[5] + 10.0*k.xkk1[4]*math.Log10(d)
        k.BLE2Dis[o][0] = float64(bl.AnchorID)
        k.BLE2Dis[o][1] = bl.Strength
        k.BLE2Dis[o][2] = math.Pow(10.0, (bl.Strength-k.xkk1[5])/(10.0*k.xkk1[4]))
        idx++
        o++
    }
    // dim expected filled in ConsHk; zeros already

    // innovations
    k.rk = make([]float64, total)
    for i := 0; i < total; i++ {
        k.rk[i] = k.yk[i] - k.ykk1[i]
    }

    Pxykk1 := matMul(Pxkk1, transpose(k.Hk)) // 6 x total
    Py0 := matMul(k.Hk, Pxykk1)              // total x total

    if k.adaptive {
        for i := 0; i < total; i++ {
            ry := k.rk[i]*k.rk[i] - Py0[i][i]
            Rymax := k.Rmax[i][i]
            Rymin := k.Rmin[i][i]
            if ry < Rymin {
                ry = Rymin
            }
            if ry > Rymax {
                k.Rk[i][i] = Rymax
            } else {
                k.Rk[i][i] = (1-k.beta)*k.Rk[i][i] + k.beta*ry
            }
        }
        k.beta = k.beta / (k.beta + k.b)
    }

    Pykk1 := matAdd(Py0, k.Rk)
    // ensure positive definiteness
    minEig := minEigen(Pykk1)
    if minEig < 1e-9 {
        add := math.Abs(minEig) + 1e-9
        for i := 0; i < len(Pykk1); i++ {
            Pykk1[i][i] += add
        }
    }
    invPy := pinv(Pykk1)

    // H_maha
    tmp := 0.0
    for i := 0; i < total; i++ {
        for j := 0; j < total; j++ {
            tmp += k.rk[i] * invPy[i][j] * k.rk[j]
        }
    }
    k.HMaha = math.Sqrt(tmp)

    Kk := matMul(Pxykk1, invPy) // 6 x total
    // update state
    incr := matVec(Kk, k.rk)
    for i := 0; i < k.n; i++ {
        k.xk[i] = k.xkk1[i] + incr[i]
    }
    // covariance
    k.Pxk = matSub(Pxkk1, matMul(Kk, matMul(Pykk1, transpose(Kk))))
    k.Pxk = scalarMat(k.Pxk, k.fading/2.0)
    k.Pykk1 = Pykk1

    // update dim constraint health
    k.Dc.RkConst(k)

    // constrain state
    for i := 0; i < k.n; i++ {
        if k.xconstrain[i] {
            k.xk[i] = clamp(k.xk[i], k.xMin[i], k.xMax[i])
        }
    }

    if !allFinite(k.xk) || !allFiniteMat(k.Pxk) {
        k.resetState()
        k.ret = -2
    } else {
        k.ret = 2
    }
}

func (k *EKF) ManagePxk() {
    consFac := PxkFacWithBle
    if k.usedMea[1] == 0 {
        consFac = PxkFacNoBle
        pvFac := consFac
        for i := 0; i < 4; i++ {
            k.Pxk[4][i] *= pvFac
            k.Pxk[i][4] *= pvFac
            k.Pxk[5][i] *= pvFac
            k.Pxk[i][5] *= pvFac
        }
    }
    maxNVar := Pow2(consFac * SigmaN0)
    maxAVar := Pow2(consFac * SigmaA0)
    if k.Pxk[4][4] > maxNVar {
        k.Pxk[4][4] = maxNVar
    }
    if k.Pxk[5][5] > maxAVar {
        k.Pxk[5][5] = maxAVar
    }
    k.Pxk = symmetrize(k.Pxk)
    // regularize
    mind := minEigen(k.Pxk)
    if mind < SReg {
        for i := 0; i < k.n; i++ {
            k.Pxk[i][i] += SReg
        }
    }
}

func (k *EKF) PredictConstrain() {
    speed := math.Hypot(k.xk[2], k.xk[3])
    if speed > 0.01 && Deceleration > 0.01 {
        scale := math.Max(speed-Deceleration*k.ts, 0.0) / speed
        k.xk[2] *= scale
        k.xk[3] *= scale
        for i := 0; i < 4; i++ {
            if i <= 1 {
                if k.Pxk[i][i] > Pow2(SigmaPos)*3 {
                    k.Pxk[i][i] = Pow2(SigmaPos) * 3
                }
            } else {
                if k.Pxk[i][i] > Pow2(SigmaVel)*3 {
                    k.Pxk[i][i] = Pow2(SigmaVel) * 3
                }
            }
        }
    }
}

// Helper matrix functions -------------------------------------------------

func zeroMat(r, c int) [][]float64 {
    m := make([][]float64, r)
    for i := 0; i < r; i++ {
        m[i] = make([]float64, c)
    }
    return m
}

func identity(n int) [][]float64 {
    m := zeroMat(n, n)
    for i := 0; i < n; i++ {
        m[i][i] = 1
    }
    return m
}

func matAdd(a, b [][]float64) [][]float64 {
    r := len(a)
    c := len(a[0])
    out := zeroMat(r, c)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            out[i][j] = a[i][j] + b[i][j]
        }
    }
    return out
}

func matSub(a, b [][]float64) [][]float64 {
    r := len(a)
    c := len(a[0])
    out := zeroMat(r, c)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            out[i][j] = a[i][j] - b[i][j]
        }
    }
    return out
}

func matMul(a, b [][]float64) [][]float64 {
    r := len(a)
    c := len(b[0])
    k := len(a[0])
    out := zeroMat(r, c)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            sum := 0.0
            for t := 0; t < k; t++ {
                sum += a[i][t] * b[t][j]
            }
            out[i][j] = sum
        }
    }
    return out
}

func matVec(a [][]float64, v []float64) []float64 {
    r := len(a)
    out := make([]float64, r)
    for i := 0; i < r; i++ {
        sum := 0.0
        for j := 0; j < len(v); j++ {
            sum += a[i][j] * v[j]
        }
        out[i] = sum
    }
    return out
}

func transpose(a [][]float64) [][]float64 {
    r := len(a)
    c := len(a[0])
    out := zeroMat(c, r)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            out[j][i] = a[i][j]
        }
    }
    return out
}

func rank2(m [][]float64) int {
    // for 2x2 matrix
    det := m[0][0]*m[1][1] - m[0][1]*m[1][0]
    if math.Abs(det) < 1e-9 {
        return 1
    }
    return 2
}

func invert2x2(m [][]float64) [][]float64 {
    det := m[0][0]*m[1][1] - m[0][1]*m[1][0]
    if math.Abs(det) < 1e-12 {
        det = 1e-12
    }
    inv := [][]float64{{m[1][1] / det, -m[0][1] / det}, {-m[1][0] / det, m[0][0] / det}}
    return inv
}

func scalarMat(a [][]float64, s float64) [][]float64 {
    r := len(a)
    c := len(a[0])
    out := zeroMat(r, c)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            out[i][j] = a[i][j] * s
        }
    }
    return out
}

func symmetrize(a [][]float64) [][]float64 {
    r := len(a)
    c := len(a[0])
    out := zeroMat(r, c)
    for i := 0; i < r; i++ {
        for j := 0; j < c; j++ {
            out[i][j] = 0.5 * (a[i][j] + a[j][i])
        }
    }
    return out
}

func minEigen(a [][]float64) float64 {
    // simple power iteration for smallest eigenvalue using Rayleigh quotient & inverse iteration fallback
    n := len(a)
    if n == 0 {
        return 0
    }
    // estimate largest eigenvalue via power iteration, then Gershgorin for min bound
    v := make([]float64, n)
    for i := 0; i < n; i++ {
        v[i] = 1.0 / float64(n)
    }
    for it := 0; it < 20; it++ {
        v = matVec(a, v)
        norm := 0.0
        for _, x := range v {
            norm += x * x
        }
        norm = math.Sqrt(norm)
        if norm < 1e-12 {
            break
        }
        for i := range v {
            v[i] /= norm
        }
    }
    // Rayleigh quotient
    num := 0.0
    for i := 0; i < n; i++ {
        for j := 0; j < n; j++ {
            num += v[i] * a[i][j] * v[j]
        }
    }
    // Gershgorin discs lower bound
    minDisc := num
    for i := 0; i < n; i++ {
        sum := 0.0
        for j := 0; j < n; j++ {
            if i == j {
                continue
            }
            sum += math.Abs(a[i][j])
        }
        disc := a[i][i] - sum
        if disc < minDisc {
            minDisc = disc
        }
    }
    return minDisc
}

func allFinite(v []float64) bool {
    for _, x := range v {
        if math.IsNaN(x) || math.IsInf(x, 0) {
            return false
        }
    }
    return true
}

func allFiniteMat(m [][]float64) bool {
    for i := 0; i < len(m); i++ {
        for j := 0; j < len(m[i]); j++ {
            if math.IsNaN(m[i][j]) || math.IsInf(m[i][j], 0) {
                return false
            }
        }
    }
    return true
}

