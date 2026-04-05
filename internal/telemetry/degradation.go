package telemetry

import (
	"math"
	"sync"

	"github.com/nikitadada/nil-loader/internal/model"
)

type DegradationResult struct {
	Detected       bool    `json:"detected"`
	DegradationRPS float64 `json:"degradationRps"`
	RecommendedRPS float64 `json:"recommendedRps"`
	// MaxStableRPS is the last stable throughput capped at DegradationRPS (avoids showing a higher RPS after load drops).
	MaxStableRPS float64 `json:"maxStableRps"`
	Reason       string  `json:"reason"`
}

type DegradationDetector struct {
	mu            sync.RWMutex
	baselineP99   float64
	baselineSum   float64
	baselineCount int
	result        DegradationResult
	snapshots     []model.MetricsSnapshot
}

const (
	baselineWindow     = 5
	p99GrowthFactor    = 2.5  // p99 вырос в 2.5x от baseline
	errorRateThreshold = 15.0 // интервальный error rate > 15%
	recommendedFactor  = 0.8
)

func NewDegradationDetector() *DegradationDetector {
	return &DegradationDetector{}
}

func (d *DegradationDetector) Analyze(snap model.MetricsSnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.snapshots = append(d.snapshots, snap)

	if snap.ActualRPS <= 0 {
		return
	}

	// Собираем baseline из первых N непустых snapshot'ов
	if d.baselineCount < baselineWindow {
		if snap.P99 > 0 {
			d.baselineSum += snap.P99
			d.baselineCount++
			d.baselineP99 = d.baselineSum / float64(d.baselineCount)
		}
		if snap.ActualRPS > d.result.MaxStableRPS {
			d.result.MaxStableRPS = snap.ActualRPS
		}
		return
	}

	if d.result.Detected {
		return
	}

	// Интервальный error rate
	intervalTotal := snap.IntervalSuccess + snap.IntervalErrors
	intervalErrRate := 0.0
	if intervalTotal > 0 {
		intervalErrRate = float64(snap.IntervalErrors) / float64(intervalTotal) * 100
	}

	p99Degraded := d.baselineP99 > 0 && snap.P99 > d.baselineP99*p99GrowthFactor
	errorDegraded := intervalErrRate > errorRateThreshold

	if !p99Degraded && !errorDegraded {
		if snap.ActualRPS > d.result.MaxStableRPS {
			d.result.MaxStableRPS = snap.ActualRPS
		}
		return
	}

	reason := ""
	if p99Degraded && errorDegraded {
		reason = "p99 latency and error rate exceeded thresholds"
	} else if p99Degraded {
		reason = "p99 latency exceeded threshold"
	} else {
		reason = "error rate exceeded threshold"
	}

	lastStable := d.findLastStableRPS()
	safeBase := math.Min(lastStable, snap.ActualRPS)

	d.result = DegradationResult{
		Detected:       true,
		DegradationRPS: snap.ActualRPS,
		RecommendedRPS: safeBase * recommendedFactor,
		MaxStableRPS:   safeBase,
		Reason:         reason,
	}
}

func (d *DegradationDetector) findLastStableRPS() float64 {
	for i := len(d.snapshots) - 2; i >= 0; i-- {
		s := d.snapshots[i]
		if s.ActualRPS <= 0 {
			continue
		}
		intervalTotal := s.IntervalSuccess + s.IntervalErrors
		if intervalTotal <= 0 {
			continue
		}
		errRate := float64(s.IntervalErrors) / float64(intervalTotal) * 100
		p99Ok := d.baselineP99 <= 0 || s.P99 < d.baselineP99*p99GrowthFactor
		if errRate < errorRateThreshold && p99Ok {
			return s.ActualRPS
		}
	}
	if len(d.snapshots) > 0 {
		fallback := d.snapshots[0].ActualRPS
		if fallback > 0 {
			return fallback
		}
	}
	return 0
}

func (d *DegradationDetector) GetResult() DegradationResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.result
}

func (d *DegradationDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.baselineP99 = 0
	d.baselineSum = 0
	d.baselineCount = 0
	d.result = DegradationResult{}
	d.snapshots = nil
}
