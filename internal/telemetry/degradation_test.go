package telemetry

import (
	"math"
	"testing"

	"github.com/nikitadada/nil-loader/internal/model"
)

func TestDegradationDetector_UsesLastStableBeforeDegradation(t *testing.T) {
	t.Parallel()

	d := NewDegradationDetector()

	feedSnapshots(d, []model.MetricsSnapshot{
		stableSnapshot(100, 100),
		stableSnapshot(120, 100),
		stableSnapshot(140, 100),
		stableSnapshot(160, 100),
		stableSnapshot(180, 100),
		stableSnapshot(493, 120), // исторический максимум stable
		stableSnapshot(300, 110), // последняя stable перед деградацией
		degradedByErrorsSnapshot(381, 120, 300, 81),
	})

	res := d.GetResult()
	if !res.Detected {
		t.Fatalf("expected degradation to be detected")
	}
	assertFloatEqual(t, res.DegradationRPS, 381)
	assertFloatEqual(t, res.MaxStableRPS, 300)
	assertFloatEqual(t, res.RecommendedRPS, 240)
	assertDegradationInvariants(t, res)
}

func TestDegradationDetector_MonotonicGrowth(t *testing.T) {
	t.Parallel()

	d := NewDegradationDetector()

	feedSnapshots(d, []model.MetricsSnapshot{
		stableSnapshot(100, 100),
		stableSnapshot(120, 100),
		stableSnapshot(140, 100),
		stableSnapshot(160, 100),
		stableSnapshot(180, 100),
		stableSnapshot(220, 110),
		stableSnapshot(300, 120),
		degradedByErrorsSnapshot(360, 120, 300, 60),
	})

	res := d.GetResult()
	if !res.Detected {
		t.Fatalf("expected degradation to be detected")
	}
	assertFloatEqual(t, res.DegradationRPS, 360)
	assertFloatEqual(t, res.MaxStableRPS, 300)
	assertFloatEqual(t, res.RecommendedRPS, 240)
	assertDegradationInvariants(t, res)
}

func TestDegradationDetector_FallbackWhenNoStableInterval(t *testing.T) {
	t.Parallel()

	d := NewDegradationDetector()

	feedSnapshots(d, []model.MetricsSnapshot{
		noIntervalSnapshot(100),
		noIntervalSnapshot(120),
		noIntervalSnapshot(140),
		noIntervalSnapshot(160),
		noIntervalSnapshot(180),
		degradedByErrorsSnapshot(80, 120, 60, 20),
	})

	res := d.GetResult()
	if !res.Detected {
		t.Fatalf("expected degradation to be detected")
	}
	assertFloatEqual(t, res.DegradationRPS, 80)
	assertFloatEqual(t, res.MaxStableRPS, 80) // min(fallback 100, degradation 80)
	assertFloatEqual(t, res.RecommendedRPS, 64)
	assertDegradationInvariants(t, res)
}

func TestDegradationDetector_LoadDropHigherLastStableThanDegradation(t *testing.T) {
	t.Parallel()

	d := NewDegradationDetector()

	feedSnapshots(d, []model.MetricsSnapshot{
		stableSnapshot(100, 100),
		stableSnapshot(120, 100),
		stableSnapshot(140, 100),
		stableSnapshot(160, 100),
		stableSnapshot(180, 100),
		stableSnapshot(1101, 100),
		degradedByErrorsSnapshot(795, 120, 300, 81),
	})

	res := d.GetResult()
	if !res.Detected {
		t.Fatalf("expected degradation to be detected")
	}
	assertFloatEqual(t, res.DegradationRPS, 795)
	assertFloatEqual(t, res.MaxStableRPS, 795)
	assertFloatEqual(t, res.RecommendedRPS, 795*recommendedFactor)
	assertDegradationInvariants(t, res)
}

func feedSnapshots(d *DegradationDetector, snapshots []model.MetricsSnapshot) {
	for _, snap := range snapshots {
		d.Analyze(snap)
	}
}

func stableSnapshot(actualRPS float64, p99 float64) model.MetricsSnapshot {
	success := int64(actualRPS)
	return model.MetricsSnapshot{
		ActualRPS:       actualRPS,
		P99:             p99,
		IntervalSuccess: success,
		IntervalErrors:  0,
	}
}

func degradedByErrorsSnapshot(actualRPS float64, p99 float64, intervalSuccess int64, intervalErrors int64) model.MetricsSnapshot {
	return model.MetricsSnapshot{
		ActualRPS:       actualRPS,
		P99:             p99,
		IntervalSuccess: intervalSuccess,
		IntervalErrors:  intervalErrors,
	}
}

func noIntervalSnapshot(actualRPS float64) model.MetricsSnapshot {
	return model.MetricsSnapshot{
		ActualRPS:       actualRPS,
		P99:             100,
		IntervalSuccess: 0,
		IntervalErrors:  0,
	}
}

func assertFloatEqual(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected value: got %.10f, want %.10f", got, want)
	}
}

func assertDegradationInvariants(t *testing.T, res DegradationResult) {
	t.Helper()
	if res.DegradationRPS <= 0 {
		return
	}
	if res.MaxStableRPS > res.DegradationRPS+1e-9 {
		t.Fatalf("max stable RPS must be <= degradation RPS: got %.2f > %.2f", res.MaxStableRPS, res.DegradationRPS)
	}
	if res.RecommendedRPS >= res.DegradationRPS-1e-9 {
		t.Fatalf("recommended RPS must be lower than degradation RPS: got %.2f >= %.2f", res.RecommendedRPS, res.DegradationRPS)
	}
}
