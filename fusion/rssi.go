package fusion

import "math"

// BLERssi converts between RSSI and range following C++ logic.
type BLERssi struct {
    Factor        float64
    AdjustRSSI    float64
    MaxRSSI       int
    RssiThresh1   int
    RssiThresh2   int
    SideLength    int
    HypotenuseLen float64
    ranges        []int
}

func NewBLERssi(factor float64, adjust float64, deploymentInterval int) *BLERssi {
    r := &BLERssi{}
    r.Init(factor, adjust, deploymentInterval)
    return r
}

func (r *BLERssi) Init(f, adjust float64, intrDist int) {
    r.Factor = f
    r.AdjustRSSI = adjust
    r.MaxRSSI = r.range2rssi(intrDist + 700)
    r.RssiThresh1 = r.range2rssi(intrDist + 400)
    r.RssiThresh2 = r.range2rssi(intrDist + 400)
    r.SideLength = intrDist + 400
    r.HypotenuseLen = float64(intrDist+700) * 1.4142135623730951
    r.ranges = make([]int, r.MaxRSSI+int(math.Abs(r.AdjustRSSI))+1)
    for i := range r.ranges {
        val := r.rssi2rangeRaw(i - int(r.AdjustRSSI))
        r.ranges[i] = val
    }
}

func (r *BLERssi) range2rssi(dist int) int {
    if dist <= 100 {
        return -int(r.AdjustRSSI)
    }
    return int(math.Ceil(math.Log10(float64(dist)*0.01)*10.0*r.Factor - r.AdjustRSSI))
}

func (r *BLERssi) rssi2rangeRaw(str int) int {
    val := float64(str) + r.AdjustRSSI
    if val < 0 {
        return 100
    }
    return int(math.Round(100.0 * math.Pow(10.0, val/(10.0*r.Factor))))
}

func (r *BLERssi) Rssi2Range(strength int) int {
    idx := strength + int(r.AdjustRSSI)
    if idx >= 0 && idx < len(r.ranges) {
        return r.ranges[idx]
    }
    return r.rssi2rangeRaw(strength)
}

func (r *BLERssi) ValidRssi(strength int) bool {
    return strength <= r.MaxRSSI
}

func (r *BLERssi) ValidRssi1(strength int) bool { return strength <= r.RssiThresh1 }
func (r *BLERssi) ValidRssi2(strength int) bool { return strength <= r.RssiThresh2 }

// StrengthFromDbm converts signed dBm to positive strength used by engine.
func (r *BLERssi) StrengthFromDbm(dbm int) int {
    if dbm >= 0 {
        return dbm
    }
    return -dbm
}

