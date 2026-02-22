package telemetry

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/nikitadada/nil-loader/internal/model"
)

type Collector struct {
	mu        sync.Mutex
	histogram *hdrhistogram.Histogram
	errors    map[string]*atomic.Int64
	success   atomic.Int64
	errCount  atomic.Int64
	requests  atomic.Int64

	intervalHist    *hdrhistogram.Histogram
	intervalSuccess atomic.Int64
	intervalErrors  atomic.Int64

	targetRPS float64

	subscribers []chan model.MetricsSnapshot
	subMu       sync.RWMutex

	errorLog    []model.ErrorEntry
	errorLogMu  sync.Mutex
	errorSubs   []chan model.ErrorEntry
	errorSubsMu sync.RWMutex
}

func NewCollector() *Collector {
	return &Collector{
		histogram:    hdrhistogram.New(1, 30000, 3), // 1µs to 30s, 3 significant digits
		intervalHist: hdrhistogram.New(1, 30000, 3),
		errors:       make(map[string]*atomic.Int64),
		errorLog:     make([]model.ErrorEntry, 0),
	}
}

func (c *Collector) RecordSuccess(latencyMs int64) {
	c.mu.Lock()
	_ = c.histogram.RecordValue(latencyMs)
	_ = c.intervalHist.RecordValue(latencyMs)
	c.mu.Unlock()
	c.success.Add(1)
	c.intervalSuccess.Add(1)
	c.requests.Add(1)
}

func (c *Collector) RecordError(code string, message string) {
	c.errCount.Add(1)
	c.intervalErrors.Add(1)
	c.requests.Add(1)

	c.mu.Lock()
	counter, ok := c.errors[code]
	if !ok {
		counter = &atomic.Int64{}
		c.errors[code] = counter
	}
	c.mu.Unlock()
	counter.Add(1)

	entry := model.ErrorEntry{
		Timestamp: time.Now(),
		Code:      code,
		Message:   message,
	}

	c.errorLogMu.Lock()
	c.errorLog = append(c.errorLog, entry)
	if len(c.errorLog) > 1000 {
		c.errorLog = c.errorLog[len(c.errorLog)-500:]
	}
	c.errorLogMu.Unlock()

	c.errorSubsMu.RLock()
	for _, ch := range c.errorSubs {
		select {
		case ch <- entry:
		default:
		}
	}
	c.errorSubsMu.RUnlock()
}

func (c *Collector) SetTargetRPS(rps float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetRPS = rps
}

func (c *Collector) Snapshot() model.MetricsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	actualRPS := float64(c.intervalSuccess.Load() + c.intervalErrors.Load())

	errMap := make(map[string]int64)
	for code, cnt := range c.errors {
		errMap[code] = cnt.Load()
	}

	intSuccess := c.intervalSuccess.Load()
	intErrors := c.intervalErrors.Load()

	snap := model.MetricsSnapshot{
		Timestamp:       time.Now(),
		P50:             float64(c.intervalHist.ValueAtQuantile(50)),
		P95:             float64(c.intervalHist.ValueAtQuantile(95)),
		P99:             float64(c.intervalHist.ValueAtQuantile(99)),
		TargetRPS:       c.targetRPS,
		ActualRPS:       actualRPS,
		SuccessNum:      c.success.Load(),
		ErrorNum:        c.errCount.Load(),
		IntervalSuccess: intSuccess,
		IntervalErrors:  intErrors,
		Errors:          errMap,
	}

	c.intervalHist.Reset()
	c.intervalSuccess.Store(0)
	c.intervalErrors.Store(0)

	return snap
}

func (c *Collector) Subscribe() chan model.MetricsSnapshot {
	ch := make(chan model.MetricsSnapshot, 64)
	c.subMu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.subMu.Unlock()
	return ch
}

func (c *Collector) Unsubscribe(ch chan model.MetricsSnapshot) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, s := range c.subscribers {
		if s == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (c *Collector) SubscribeErrors() chan model.ErrorEntry {
	ch := make(chan model.ErrorEntry, 128)
	c.errorSubsMu.Lock()
	c.errorSubs = append(c.errorSubs, ch)
	c.errorSubsMu.Unlock()
	return ch
}

func (c *Collector) UnsubscribeErrors(ch chan model.ErrorEntry) {
	c.errorSubsMu.Lock()
	defer c.errorSubsMu.Unlock()
	for i, s := range c.errorSubs {
		if s == ch {
			c.errorSubs = append(c.errorSubs[:i], c.errorSubs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (c *Collector) Broadcast(snap model.MetricsSnapshot) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	for _, ch := range c.subscribers {
		select {
		case ch <- snap:
		default:
		}
	}
}

func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.histogram.Reset()
	c.intervalHist.Reset()
	c.errors = make(map[string]*atomic.Int64)
	c.success.Store(0)
	c.errCount.Store(0)
	c.requests.Store(0)
	c.intervalSuccess.Store(0)
	c.intervalErrors.Store(0)
	c.targetRPS = 0

	c.errorLogMu.Lock()
	c.errorLog = make([]model.ErrorEntry, 0)
	c.errorLogMu.Unlock()
}
