package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nikitadada/nil-loader/internal/grpcclient"
	"github.com/nikitadada/nil-loader/internal/model"
	"github.com/nikitadada/nil-loader/internal/payload"
	"github.com/nikitadada/nil-loader/internal/telemetry"
	"google.golang.org/grpc/status"
)

type Engine struct {
	state     *model.TestState
	collector *telemetry.Collector
	detector  *telemetry.DegradationDetector
	cancel    context.CancelFunc
	done      chan struct{}

	logCh chan string
	mu    sync.Mutex
}

func NewEngine(state *model.TestState, collector *telemetry.Collector, detector *telemetry.DegradationDetector) *Engine {
	return &Engine{
		state:     state,
		collector: collector,
		detector:  detector,
		logCh:     make(chan string, 256),
	}
}

func (e *Engine) Detector() *telemetry.DegradationDetector {
	return e.detector
}

func (e *Engine) LogChannel() <-chan string {
	return e.logCh
}

func (e *Engine) emitLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
	select {
	case e.logCh <- msg:
	default:
	}
}

func (e *Engine) Start(cfg *model.TestConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state.GetStatus() == model.TestStatusRunning || e.state.GetStatus() == model.TestStatusWarmup {
		return fmt.Errorf("test already running")
	}

	e.collector.Reset()
	e.detector.Reset()
	e.state.Reset(cfg)

	gen, err := payload.NewGenerator(cfg.PayloadTemplate, cfg.CSVData)
	if err != nil {
		return fmt.Errorf("create payload generator: %w", err)
	}

	caller, err := grpcclient.NewCaller(
		cfg.Target,
		cfg.UseReflection,
		cfg.Service,
		cfg.Method,
		cfg.ProtoContent,
	)
	if err != nil {
		return fmt.Errorf("create grpc caller: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.done = make(chan struct{})

	go e.run(ctx, cfg, caller, gen)
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
}

func (e *Engine) Wait() {
	if e.done != nil {
		<-e.done
	}
}

func (e *Engine) run(ctx context.Context, cfg *model.TestConfig, caller *grpcclient.Caller, gen *payload.Generator) {
	defer close(e.done)
	defer caller.Close()

	e.state.SetStatus(model.TestStatusWarmup)
	e.state.StartedAt = time.Now()
	e.emitLog("[ENGINE] Warmup phase — connecting to %s", cfg.Target)

	time.Sleep(1 * time.Second)

	e.state.SetStatus(model.TestStatusRunning)
	e.emitLog("[ENGINE] Test started: %s/%s, profile=%s, duration=%ds",
		cfg.Service, cfg.Method, cfg.LoadProfile, cfg.Duration)

	maxWorkers := cfg.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 200
	}
	workerSem := make(chan struct{}, maxWorkers)

	duration := time.Duration(cfg.Duration) * time.Second
	deadline := time.Now().Add(duration)

	tickerInterval := time.Millisecond * 100
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	metricsTicker := time.NewTicker(1 * time.Second)
	defer metricsTicker.Stop()

	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			e.emitLog("[ENGINE] Test stopped by user")
			goto cleanup
		case <-metricsTicker.C:
			snap := e.collector.Snapshot()
			e.state.AddSnapshot(snap)
			e.detector.Analyze(snap)
			e.collector.Broadcast(snap)
		case t := <-ticker.C:
			if t.After(deadline) {
				e.emitLog("[ENGINE] Duration reached, finishing...")
				goto cleanup
			}

			currentRPS := e.calculateRPS(cfg, time.Since(e.state.StartedAt).Seconds()-1)
			e.collector.SetTargetRPS(currentRPS)

			requestsPerTick := currentRPS * tickerInterval.Seconds()
			count := int(requestsPerTick)

			for i := 0; i < count; i++ {
				select {
				case <-ctx.Done():
					goto cleanup
				case workerSem <- struct{}{}:
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { <-workerSem }()
						e.executeRequest(ctx, caller, gen)
					}()
				default:
				}
			}
		}
	}

cleanup:
	e.state.SetStatus(model.TestStatusStopping)
	e.emitLog("[ENGINE] Waiting for in-flight requests...")
	wg.Wait()

	snap := e.collector.Snapshot()
	e.state.AddSnapshot(snap)
	e.detector.Analyze(snap)
	e.collector.Broadcast(snap)

	e.state.SetStatus(model.TestStatusFinished)
	e.emitLog("[ENGINE] Test finished. Total success=%d, errors=%d",
		e.state.GetState().Snapshots[len(e.state.GetState().Snapshots)-1].SuccessNum,
		e.state.GetState().Snapshots[len(e.state.GetState().Snapshots)-1].ErrorNum)
}

func (e *Engine) calculateRPS(cfg *model.TestConfig, elapsedSec float64) float64 {
	switch cfg.LoadProfile {
	case model.LoadProfileConstant:
		return float64(cfg.StartRPS)
	case model.LoadProfileRamping:
		totalSec := float64(cfg.Duration)
		if totalSec <= 0 {
			return float64(cfg.StartRPS)
		}
		progress := elapsedSec / totalSec
		if progress > 1 {
			progress = 1
		}
		if progress < 0 {
			progress = 0
		}
		return float64(cfg.StartRPS) + (float64(cfg.EndRPS)-float64(cfg.StartRPS))*progress
	default:
		return float64(cfg.StartRPS)
	}
}

func (e *Engine) executeRequest(ctx context.Context, caller *grpcclient.Caller, gen *payload.Generator) {
	data, err := gen.Generate()
	if err != nil {
		e.collector.RecordError("PAYLOAD_ERROR", err.Error())
		e.state.AddError(model.ErrorEntry{
			Timestamp: time.Now(),
			Code:      "PAYLOAD_ERROR",
			Message:   err.Error(),
		})
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	latency, err := caller.Call(callCtx, data)
	latencyMs := latency.Milliseconds()

	if err != nil {
		st, ok := status.FromError(err)
		code := "UNKNOWN"
		msg := err.Error()
		if ok {
			code = st.Code().String()
			msg = st.Message()
		}
		e.collector.RecordError(code, msg)
		e.state.AddError(model.ErrorEntry{
			Timestamp: time.Now(),
			Code:      code,
			Message:   msg,
		})
		return
	}

	e.collector.RecordSuccess(latencyMs)
}
