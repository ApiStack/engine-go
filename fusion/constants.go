package fusion

// Engine constants mirrored from the C++/Python implementations.
var (
	PathLossExp = [3]float64{2.5, 3.0, 3.5}
	DeltaA      = [3]float64{7.0, 8.0, 9.0}
)

const (
	MaxVel            = 1.5
	ToFErr            = 0.4
	BleErr            = 3.0
	DimErr            = 0.2
	GNSSErr           = 0.5
	SigmaAcc          = 0.08
	SigmaN            = 1e-3
	SigmaA            = 1e-2
	SigmaPos          = 5.0
	SigmaVel          = 1.0
	SigmaN0           = 0.1
	SigmaA0           = 1.0
	MinDistance       = 0.1
	StateDim          = 6
	MaxMeaDim         = 12
	UseAdaptive       = true
	Fading            = 1.0
	Deceleration      = 0.3
	BetaInit          = 1.0
	BetaB             = 0.98
	SReg              = 1e-9
	PxkFacWithBle     = 3.0
	PxkFacNoBle       = 0.5
	DisLimit          = 10.0
	EndpointLimit     = 3.0
	HistoryLen        = 5
	MapMargin         = 30.0
	MaxJumpPerStep    = 80.0
	KinematicSpeedMax = 5.0 // m/s allowed between valid outputs
	KinematicSlack    = 5.0 // meters slack to tolerate jitter/start
)

var PxkFac = [2]float64{PxkFacWithBle, PxkFacNoBle}

// Earth constants kept for completeness (not currently used).
const (
	Re  = 6378137.0
	E2  = 0.006694385
	Ep2 = 0.0067395
)

// HDOP sanity cap.
const HDOPMax = 50.0

// Outdoor layer ID used by the engine.
const OutdoorLayer = 1

// Debug switch kept for parity with Python.
const Debug = false

// ChiSquare tables (p=0.99 and p=0.95) for df 1..10, used in DimConstrain.
var chi2_01 = []float64{6.6349, 9.2103, 11.3449, 13.2767, 15.0863, 16.8119, 18.4753, 20.0902, 21.6660, 23.2093}
var chi2_05 = []float64{3.8415, 5.9915, 7.8147, 9.4877, 11.0705, 12.5916, 14.0671, 15.5073, 16.9189, 18.3070}

// Clamp returns x within [min, max].
func clamp(x, min, max float64) float64 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

// Max returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MaxF returns the larger of two float64.
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// MinF returns the smaller of two float64.
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// AbsF absolute value.
func AbsF(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

// Pow2 returns squared value.
func Pow2(x float64) float64 { return x * x }

// Ln10 cached natural log of 10.
const Ln10 = 2.302585092994046

// DB10 convenience.
func DB10(x float64) float64 { return 10.0 * x }
