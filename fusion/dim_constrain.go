package fusion

import "math"

type DimConstrain struct {
    hisLen         int
    rkExamed       [][]float64 // [hisLen][3]
    dimConsedFlags [][]int     // [hisLen][2]
    dimConsedDis   [][]float64 // [hisLen][2]
    dimConsUse     []bool
    dimRkOverLim   bool
}

func NewDimConstrain(hisLen int) *DimConstrain {
    dc := &DimConstrain{
        hisLen:         hisLen,
        rkExamed:       make([][]float64, hisLen),
        dimConsedFlags: make([][]int, hisLen),
        dimConsedDis:   make([][]float64, hisLen),
    }
    for i := 0; i < hisLen; i++ {
        dc.rkExamed[i] = []float64{0, 0, 0}
        dc.dimConsedFlags[i] = []int{0, 0}
        dc.dimConsedDis[i] = []float64{0, 0}
    }
    return dc
}

func (d *DimConstrain) distanceCal(point [2]float64, dimPos DimMat) (float64, float64) {
    if len(dimPos) == 1 {
        dx := point[0] - dimPos[0][0]
        dy := point[1] - dimPos[0][1]
        return math.Hypot(dx, dy), 0.0
    } else if len(dimPos) == 2 {
        p1 := dimPos[0]
        p2 := dimPos[1]
        a := p2[1] - p1[1]
        b := p1[0] - p2[0]
        c := p2[0]*p1[1] - p1[0]*p2[1]
        norm := math.Hypot(a, b)
        if norm < MinDistance {
            norm = MinDistance
        }
        distLine := math.Abs(a*point[0]+b*point[1]+c) / norm
        t := ((point[0]-p1[0])*(p2[0]-p1[0]) + (point[1]-p1[1])*(p2[1]-p1[1])) / (Pow2(p2[0]-p1[0]) + Pow2(p2[1]-p1[1]))
        distEP := 0.0
        if t < 0 {
            distEP = math.Hypot(point[0]-p1[0], point[1]-p1[1])
        } else if t > 1 {
            distEP = math.Hypot(point[0]-p2[0], point[1]-p2[1])
        }
        return distLine, distEP
    }
    return math.Inf(1), math.Inf(1)
}

// DimConsDeter determines which dimension constraints to enable for this sample.
func (d *DimConstrain) DimConsDeter(sample *EKFSample, ekf *EKF) {
    hasData := len(sample.DimPos) > 0
    dimType := []int{0, 0}
    d.dimConsUse = make([]bool, len(sample.DimPos))
    if hasData && !d.dimRkOverLim {
        for i, mat := range sample.DimPos {
            if i >= len(d.dimConsedDis) {
                extra := i - len(d.dimConsedDis) + 1
                for k := 0; k < extra; k++ {
                    d.dimConsedDis = append(d.dimConsedDis, []float64{0, 0})
                    d.dimConsedFlags = append(d.dimConsedFlags, []int{0, 0})
                }
            }
            if len(mat) == 0 {
                d.dimConsUse[i] = false
                continue
            }
            distLine, distEP := d.distanceCal([2]float64{ekf.xk[0], ekf.xk[1]}, mat)
            if len(mat) == 1 {
                dimType[0] = 1
            } else if len(mat) == 2 {
                dimType[1] = 1
            }
            if distLine > DisLimit || distEP > EndpointLimit {
                d.dimConsUse[i] = false
            } else {
                d.dimConsedDis[i][0] = distLine
                d.dimConsedDis[i][1] = distEP
                ekf.usedMea[3]++
                d.dimConsUse[i] = true
            }
        }
    }
    if len(d.dimConsedFlags) > 1 {
        copy(d.dimConsedFlags[1:], d.dimConsedFlags[:len(d.dimConsedFlags)-1])
    }
    d.dimConsedFlags[0][0] = dimType[0]
    d.dimConsedFlags[0][1] = dimType[1]
}

// ConsHk builds virtual measurement rows for enabled dimension constraints.
func (d *DimConstrain) ConsHk(sample *EKFSample, ekf *EKF) {
    if ekf.usedMea[3] == 0 {
        return
    }
    meaSize := ekf.usedMea[0] + ekf.usedMea[1] + ekf.usedMea[2]
    g1 := 0
    for g, mat := range sample.DimPos {
        if !d.dimConsUse[g] {
            continue
        }
        idx := meaSize + g1
        g1++
        if len(mat) == 1 {
            dx := ekf.xkk1[0] - mat[0][0]
            dy := ekf.xkk1[1] - mat[0][1]
            dval := math.Hypot(dx, dy)
            if dval < MinDistance {
                dval = MinDistance
            }
            ekf.yk[idx] = 0.0
            ekf.ykk1[idx] = dval
            for j := 0; j < len(ekf.Hk[idx]); j++ {
                ekf.Hk[idx][j] = 0
            }
            ekf.Hk[idx][0] = dx / dval
            ekf.Hk[idx][1] = dy / dval
            fHdop := RandomModel(ekf.HDOP, "dh")
            fDis := RandomModel(d.dimConsedDis[g][0], "dd")
            ekf.Rk[idx][idx] = Pow2(DimErr * fHdop * fDis)
        } else if len(mat) == 2 {
            A := mat[1][1] - mat[0][1]
            B := mat[0][0] - mat[1][0]
            C := mat[1][0]*mat[0][1] - mat[0][0]*mat[1][1]
            norm := math.Hypot(A, B)
            if norm < MinDistance {
                norm = MinDistance
            }
            A /= norm; B /= norm; C /= norm
            ekf.yk[idx] = 0.0
            ekf.ykk1[idx] = A*ekf.xkk1[0] + B*ekf.xkk1[1] + C
            for j := 0; j < len(ekf.Hk[idx]); j++ {
                ekf.Hk[idx][j] = 0
            }
            ekf.Hk[idx][0] = A
            ekf.Hk[idx][1] = B
            fHdop := RandomModel(ekf.HDOP, "dh")
            fDis := RandomModel(d.dimConsedDis[g][0], "dd")
            ekf.Rk[idx][idx] = Pow2(DimErr * fHdop * fDis)
        }
    }
}

// RkConst updates constraint health using recent residual statistics.
func (d *DimConstrain) RkConst(ekf *EKF) {
    d.dimRkOverLim = false
    meaSize := ekf.usedMea[0] + ekf.usedMea[1] + ekf.usedMea[2]
    if meaSize == 0 {
        return
    }
    rkVec := ekf.rk[:meaSize]
    // extract Pykk_1 subset
    py := make([][]float64, meaSize)
    for i := 0; i < meaSize; i++ {
        py[i] = make([]float64, meaSize)
        copy(py[i], ekf.Pykk1[i][:meaSize])
    }
    stats := RKStatistics(meaSize, rkVec, py)
    // shift history
    copy(d.rkExamed[1:], d.rkExamed[:len(d.rkExamed)-1])
    d.rkExamed[0][0] = stats[0]
    d.rkExamed[0][1] = stats[1]
    d.rkExamed[0][2] = stats[2]

    // averages
    meanSrk := 0.0
    stdSrk := 0.0
    nisTotal := 0.0
    for i := 0; i < len(d.rkExamed); i++ {
        meanSrk += d.rkExamed[i][0]
        stdSrk += d.rkExamed[i][1]
        nisTotal += d.rkExamed[i][2]
    }
    l := float64(len(d.rkExamed))
    meanSrk /= l
    stdSrk /= l
    nisTotal /= l

    chiThr := Chi2Inv(0.99, meaSize)
    nisRatio := 0.0
    if chiThr > 0 {
        nisRatio = nisTotal / chiThr
    }
    condBias := math.Abs(meanSrk) > 0.3
    condVar := stdSrk > 0.4
    condChi := nisRatio > 1.0
    abnormal := 0
    if condBias {
        abnormal++
    }
    if condVar {
        abnormal++
    }
    if condChi {
        abnormal++
    }
    if ekf.usedMea[3] > 0 && abnormal >= 2 {
        d.dimRkOverLim = true
    }
}
