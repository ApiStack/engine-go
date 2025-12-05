package fusion

import (
    "math"
    "sort"

    "engine-go/fusion/loose"
)

type BLEMeas struct {
    AnchorID int
    RSSIDb   int
}

type TWRMeas struct {
    AnchorID int
    Range    float64
}

type FusionResult struct {
    TimestampMs int64
    X           float64
    Y           float64
    Flag        int
    UsedMea     [2]int
    NumBeacons  int
    Algo        string
    Layer       *int
}

type FusionPipeline struct {
    anchors      map[int]Anchor
    rssiModel    *BLERssi
    ekf          *EKF
    lastTS       *int64
    lastImuDist  *float64
    initialized  bool
    dimMap       map[int][]DimMat
    beaconLayer  map[int]int
    beaconDims   map[int][]DimMat
    layerManager *LayerManager
    divergeCount int
    looseFusor   *loose.Fusor
}

func NewFusionPipeline(anchors map[int]Anchor, rssi *BLERssi, dimMap map[int][]DimMat, beaconLayer map[int]int, beaconDims map[int][]DimMat, lm *LayerManager) *FusionPipeline {
	// Ensure Short ID aliases exist for lookups
	for id, a := range anchors {
		short := id & 0xFFFF
		if _, ok := anchors[short]; !ok {
			alias := a
			alias.ID = short
			anchors[short] = alias
		}
	}
	return &FusionPipeline{
		anchors:      anchors,
		rssiModel:    rssi,
		ekf:          NewEKF(),
		dimMap:       dimMap,
		beaconLayer:  beaconLayer,
		beaconDims:   beaconDims,
		layerManager: lm,
        divergeCount: 0,
        looseFusor:   loose.NewFusor(loose.DefaultConfig()),
	}
}

func (p *FusionPipeline) AddAnchor(a Anchor) {
	p.anchors[a.ID] = a
}

func (p *FusionPipeline) HasAnchor(id int) bool {
	_, ok := p.anchors[id]
	return ok
}

func (p *FusionPipeline) chooseLayer(bleMeas []BLEMeas, twrMeas []TWRMeas, currentPos [2]float64) *int {
    if p.layerManager == nil {
        return nil
    }
    var pos3 [3]float64
    if p.initialized {
        pos3 = [3]float64{currentPos[0], currentPos[1], 0}
    } else {
        xs := []float64{}
        ys := []float64{}
        for _, m := range twrMeas {
            if a, ok := p.anchors[m.AnchorID]; ok {
                xs = append(xs, a.X)
                ys = append(ys, a.Y)
            }
        }
        for _, m := range bleMeas {
            if a, ok := p.anchors[m.AnchorID]; ok {
                xs = append(xs, a.X)
                ys = append(ys, a.Y)
            }
        }
        if len(xs) > 0 && len(ys) > 0 {
            var sx, sy float64
            for _, v := range xs {
                sx += v
            }
            for _, v := range ys {
                sy += v
            }
            pos3 = [3]float64{sx / float64(len(xs)), sy / float64(len(ys)), 0}
        } else {
            pos3 = [3]float64{0, 0, 0}
        }
    }
    layer := p.layerManager.GetLayer(bleMeas, twrMeas, pos3, p.rssiModel, p.anchors)
    return layer
}

func (p *FusionPipeline) buildSample(tsMs int64, tagID int, bleMeas []BLEMeas, twrMeas []TWRMeas, tagHeight float64, layerSel *int, currentPos [2]float64, initialized bool) (*EKFSample, []DimMat) {
    bleRows := []BLERow{}
    bleEstRanges := []float64{}
    for _, m := range bleMeas {
        a, ok := p.anchors[m.AnchorID]
        if !ok {
            continue
        }
        strength := p.rssiModel.StrengthFromDbm(m.RSSIDb)
        bleRows = append(bleRows, BLERow{X: a.X, Y: a.Y, Z: a.Z, Strength: float64(strength), AnchorID: m.AnchorID, Layer: a.Layer})
        if p.rssiModel.ValidRssi(strength) {
            bleEstRanges = append(bleEstRanges, 0.01*float64(p.rssiModel.Rssi2Range(strength)))
        }
    }

    twrRows := []TWRRow{}
    for _, m := range twrMeas {
        a, ok := p.anchors[m.AnchorID]
        if !ok {
            continue
        }
        if m.Range < 0.01 || m.Range > 400.0 {
            continue
        }
        
        // Sanity Check / Gating
        if initialized {
            dist := math.Hypot(a.X-currentPos[0], a.Y-currentPos[1])
            // If measured range differs significantly from expected distance, reject it.
            // Threshold: 50m (allows for fast movement/recovery, but rejects massive outliers)
            if math.Abs(m.Range-dist) > 50.0 {
                continue
            }
        }

        if len(bleEstRanges) > 0 {
            minBle := bleEstRanges[0]
            for _, v := range bleEstRanges[1:] {
                if v < minBle {
                    minBle = v
                }
            }
            if m.Range > math.Max(30.0, 2.0*minBle) {
                continue
            }
        }
        twrRows = append(twrRows, TWRRow{X: a.X, Y: a.Y, Z: a.Z, Range: m.Range, AnchorID: m.AnchorID, Layer: a.Layer})
    }

    // dim constraints
    dimPos := []DimMat{}
    bleList := []struct {
        aid      int
        strength int
    }{}
    for _, m := range bleMeas {
        if _, ok := p.anchors[m.AnchorID]; !ok {
            continue
        }
        strength := p.rssiModel.StrengthFromDbm(m.RSSIDb)
        bleList = append(bleList, struct {
            aid      int
            strength int
        }{aid: m.AnchorID, strength: strength})
    }
    sort.Slice(bleList, func(i, j int) bool { return bleList[i].strength < bleList[j].strength })
    dimCap := 5
    for _, item := range bleList {
        if len(dimPos) >= dimCap {
            break
        }
        aid := item.aid
        if layerSel != nil {
            lay := p.beaconLayer[aid]
            if lay == 0 {
                if a, ok := p.anchors[aid]; ok {
                    lay = a.Layer
                }
            }
            if lay != 0 && lay != *layerSel {
                continue
            }
        }
        mats := p.beaconDims[aid]
        if len(mats) > 0 {
            for _, m := range mats {
                dimPos = append(dimPos, m)
                if len(dimPos) >= dimCap {
                    break
                }
            }
        } else if a, ok := p.anchors[aid]; ok {
            dimPos = append(dimPos, DimMat{{a.X, a.Y, a.Z}})
        }
    }
    if layerSel != nil && *layerSel != OutdoorLayer {
        for _, m := range p.dimMap[*layerSel] {
            if len(dimPos) >= dimCap {
                break
            }
            dimPos = append(dimPos, m)
        }
    }

    sample := &EKFSample{
        Timestamp: tsMs,
        TagID:     tagID,
        TagHeight: tagHeight,
        BLE:       bleRows,
        TWR:       twrRows,
        DimPos:    dimPos,
    }
    return sample, dimPos
}

func (p *FusionPipeline) Process(tsMs int64, tagID int, bleMeas []BLEMeas, twrMeas []TWRMeas, tagHeight float64) FusionResult {
    if p.lastTS == nil {
        p.lastTS = new(int64)
        *p.lastTS = tsMs
    }
    currentPos := [2]float64{0, 0}
    if p.initialized {
        currentPos[0] = p.ekf.xk[0]
        currentPos[1] = p.ekf.xk[1]
    }

    layerSel := p.chooseLayer(bleMeas, twrMeas, currentPos)
    sample, dimUsed := p.buildSample(tsMs, tagID, bleMeas, twrMeas, tagHeight, layerSel, currentPos, p.initialized)

    if !p.initialized && (len(sample.TWR) > 0 || len(sample.BLE) > 0) {
        if len(sample.BLE) > 0 {
            var sx, sy float64
            for _, b := range sample.BLE {
                sx += b.X
                sy += b.Y
            }
            meanX := sx / float64(len(sample.BLE))
            meanY := sy / float64(len(sample.BLE))
            p.ekf.xk[0] = meanX + 1.0
            p.ekf.xk[1] = meanY + 1.0
        } else {
            var sx, sy float64
            for _, t := range sample.TWR {
                sx += t.X
                sy += t.Y
            }
            meanX := sx / float64(len(sample.TWR))
            meanY := sy / float64(len(sample.TWR))
            p.ekf.xk[0] = meanX + 1.0
            p.ekf.xk[1] = meanY + 1.0
        }
        p.initialized = true
        p.divergeCount = 0
    }

    if tsMs <= *p.lastTS {
        tsMs = *p.lastTS + 1
    }
    dt := float64(tsMs-*p.lastTS) / 1000.0
    if dt > 30.0 {
        p.ekf.resetState()
        p.initialized = false
        *p.lastTS = tsMs
        p.divergeCount = 0
        return FusionResult{TimestampMs: tsMs, X: 0, Y: 0, Flag: -2, UsedMea: [2]int{0, 0}, NumBeacons: 0, Algo: "NA", Layer: layerSel}
    }

    p.ekf.Updt(math.Max(dt, 0.01))
    p.ekf.UpMeas(sample)
    p.ekf.KfUpdate(sample)
    *p.lastTS = tsMs
    flag := p.ekf.ret

    // Watchdog: If state covariance explodes (Sigma > 100m), reset
    // This allows large coordinates but catches filter divergence.
    if p.ekf.Pxk[0][0] > 10000.0 || p.ekf.Pxk[1][1] > 10000.0 {
        p.ekf.resetState()
        p.initialized = false
        p.divergeCount = 0
        return FusionResult{TimestampMs: tsMs, X: 0, Y: 0, Flag: -2, UsedMea: [2]int{0, 0}, NumBeacons: 0, Algo: "NA", Layer: layerSel}
    }

    // Check for divergence/rejection
    if flag == -3 {
        p.divergeCount++
        if p.divergeCount > 5 {
            p.ekf.resetState()
            p.initialized = false
            p.divergeCount = 0
            // Return reset flag
            return FusionResult{TimestampMs: tsMs, X: 0, Y: 0, Flag: -2, UsedMea: [2]int{0, 0}, NumBeacons: 0, Algo: "NA", Layer: layerSel}
        }
    } else if flag >= 0 {
        p.divergeCount = 0
    }

    if flag == 1 {
        p.ekf.PredictConstrain()
    }

    // Feed valid EKF positions to LooseFusor as "UWB Fixes"
    // This allows LooseFusor to benefit from the geometry solver of EKF
    tsSec := float64(tsMs) / 1000.0
    if flag == 2 { // 2 = Measurement Updated
        uwbFix := loose.UwbFix{X: p.ekf.xk[0], Y: p.ekf.xk[1]}
        p.looseFusor.IngestBatch(loose.SensorBatch{
            Timestamp: tsSec,
            Uwb:       &uwbFix,
        })
    }

    if p.layerManager != nil {
        curr := [3]float64{p.ekf.xk[0], p.ekf.xk[1], 0}
        chk := p.layerManager.GetLayer(bleMeas, twrMeas, curr, p.rssiModel, p.anchors)
        if chk == nil {
            flag = -1
        } else {
            layerSel = chk
        }
    }

    algo := "0D"
    for _, m := range dimUsed {
        if len(m) == 2 {
            algo = "1D"
            break
        }
    }

    used := [2]int{p.ekf.usedMea[0], p.ekf.usedMea[1]}
    
    // Use LooseFusor output if available
    outX, outY := p.ekf.xk[0], p.ekf.xk[1]
    var looseEst loose.Estimate
    if p.looseFusor.Latest(&looseEst) {
        // Use raw or smoothed? Smoothed might lag.
        // Use Raw for now to be responsive.
        if !math.IsNaN(looseEst.RawX) {
            lX, lY := looseEst.RawX, looseEst.RawY
            
            // Divergence Check: If LooseFusor drifts too far from EKF (Ground Truth),
            // snap back to EKF and reset LooseFusor.
            dist := math.Hypot(lX - p.ekf.xk[0], lY - p.ekf.xk[1])
            if dist > 20.0 {
                // Divergence detected! Trust EKF.
                // Log for debugging
                // log.Printf("Divergence: EKF(%.1f, %.1f) Loose(%.1f, %.1f) Dist=%.1f", p.ekf.xk[0], p.ekf.xk[1], lX, lY, dist)
                outX, outY = p.ekf.xk[0], p.ekf.xk[1]
                // Reset LooseFusor to snap it back
                p.looseFusor = loose.NewFusor(loose.DefaultConfig())
                // Seed new fusor with current EKF state
                p.looseFusor.IngestBatch(loose.SensorBatch{
                    Timestamp: tsSec,
                    Uwb:       &loose.UwbFix{X: outX, Y: outY},
                })
            } else {
                outX, outY = lX, lY
            }
        }
    }

    // Final Watchdog on Output
    if math.IsNaN(outX) || math.IsNaN(outY) {
         p.ekf.resetState()
         p.initialized = false
         p.divergeCount = 0
         // Reset LooseFusor too
         p.looseFusor = loose.NewFusor(loose.DefaultConfig())
         return FusionResult{TimestampMs: tsMs, X: 0, Y: 0, Flag: -2, UsedMea: [2]int{0, 0}, NumBeacons: 0, Algo: "NA", Layer: layerSel}
    }

    return FusionResult{
        TimestampMs: tsMs,
        X:           outX,
        Y:           outY,
        Flag:        flag,
        UsedMea:     used,
        NumBeacons:  len(sample.BLE) + len(sample.TWR),
        Algo:        algo,
        Layer:       layerSel,
    }
}

// ProcessIMU advances the filter using dead-reckoning distance/yaw (degrees).
// It performs a predict step with dt from last timestamp, then shifts position along yaw.
func (p *FusionPipeline) ProcessIMU(tsMs int64, distance float64, yawDeg float64) {
    if p.lastTS == nil {
        p.lastTS = new(int64)
        *p.lastTS = tsMs
        p.lastImuDist = new(float64)
        *p.lastImuDist = distance
        return
    }
    if p.lastImuDist == nil {
        p.lastImuDist = new(float64)
        *p.lastImuDist = distance
    }
    
    deltaDist := distance - *p.lastImuDist
    *p.lastImuDist = distance
    
    if tsMs <= *p.lastTS {
        tsMs = *p.lastTS + 1
    }
    dt := float64(tsMs-*p.lastTS) / 1000.0
    if dt > 30.0 {
        p.ekf.resetState()
        p.initialized = false
        *p.lastTS = tsMs
        *p.lastImuDist = distance // Reset IMU baseline
        p.divergeCount = 0
        return
    }

    // Sanity check: Ignore unrealistic jumps (e.g. > 20m/s or > 5m absolute step)
    // This prevents IMU glitches from diverging the filter.
    // Check BEFORE feeding to LooseFusor.
    if math.Abs(deltaDist) > 5.0 || (dt > 0 && math.Abs(deltaDist)/dt > 20.0) {
        return
    }

    // Feed IMU to LooseFusor
    // Note: LooseFusor handles integration internally.
    tsSec := float64(tsMs) / 1000.0
    imuRep := loose.ImuReport{
        YawDeg:       yawDeg,
        SpeedMps:     0, // Not provided, derived
        ForwardDisM:  distance,
        MotionCode:   1, // Assume moving if receiving IMU
        YawSigmaCode: 0,
        DsSigmaCode:  0,
    }
    p.looseFusor.IngestBatch(loose.SensorBatch{
        Timestamp: tsSec,
        Imu:       &imuRep,
    })

    p.ekf.Updt(math.Max(dt, 0.01))
    // predict state (no measurements)
    p.ekf.xk = matVec(p.ekf.Phikk1, p.ekf.xk)
    p.ekf.Pxk = matAdd(matMul(p.ekf.Phikk1, matMul(p.ekf.Pxk, transpose(p.ekf.Phikk1))), p.ekf.Qk)

    // apply displacement
    rad := yawDeg * math.Pi / 180.0
    dx := deltaDist * math.Cos(rad)
    dy := deltaDist * math.Sin(rad)
    p.ekf.xk[0] += dx
    p.ekf.xk[1] += dy
    
    // Clamp IMU dead-reckoning
    p.ekf.xk[0] = clamp(p.ekf.xk[0], p.ekf.xMin[0], p.ekf.xMax[0])
    p.ekf.xk[1] = clamp(p.ekf.xk[1], p.ekf.xMin[1], p.ekf.xMax[1])

    // Watchdog: If state covariance explodes (Sigma > 100m), reset
    if p.ekf.Pxk[0][0] > 10000.0 || p.ekf.Pxk[1][1] > 10000.0 {
        p.ekf.resetState()
        p.initialized = false
        p.divergeCount = 0
        return
    }

    if dt > 0.0 {
        vx := dx / dt
        vy := dy / dt
        // clamp velocities
        speed := math.Hypot(vx, vy)
        if speed > MaxVel {
            scale := MaxVel / speed
            vx *= scale
            vy *= scale
        }
        p.ekf.xk[2] = vx
        p.ekf.xk[3] = vy
    }
    *p.lastTS = tsMs
    // Do NOT set p.initialized = true here.
    // IMU is relative. We need TWR/BLE to establish absolute position.
}
