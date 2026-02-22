package model

import (
	"sync"
	"time"
)

type LoadProfile string

const (
	LoadProfileConstant LoadProfile = "constant"
	LoadProfileRamping  LoadProfile = "ramping"
)

type TestStatus string

const (
	TestStatusIdle     TestStatus = "idle"
	TestStatusWarmup   TestStatus = "warmup"
	TestStatusRunning  TestStatus = "running"
	TestStatusStopping TestStatus = "stopping"
	TestStatusFinished TestStatus = "finished"
)

type TestConfig struct {
	Target      string      `json:"target"`
	Service     string      `json:"service"`
	Method      string      `json:"method"`
	LoadProfile LoadProfile `json:"loadProfile"`
	StartRPS    int         `json:"startRps"`
	EndRPS      int         `json:"endRps"`
	Duration    int         `json:"duration"` // seconds
	MaxWorkers  int         `json:"maxWorkers"`

	ProtoFile    []byte `json:"protoFile,omitempty"`
	ProtoContent string `json:"protoContent,omitempty"`

	PayloadTemplate string `json:"payloadTemplate"`
	CSVData         string `json:"csvData,omitempty"`

	UseReflection bool `json:"useReflection"`
}

type MetricsSnapshot struct {
	Timestamp       time.Time        `json:"timestamp"`
	P50             float64          `json:"p50"`
	P95             float64          `json:"p95"`
	P99             float64          `json:"p99"`
	TargetRPS       float64          `json:"targetRps"`
	ActualRPS       float64          `json:"actualRps"`
	SuccessNum      int64            `json:"successNum"`
	ErrorNum        int64            `json:"errorNum"`
	IntervalSuccess int64            `json:"intervalSuccess"`
	IntervalErrors  int64            `json:"intervalErrors"`
	Errors          map[string]int64 `json:"errors"`
}

type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
}

type TestStateView struct {
	Status    TestStatus        `json:"status"`
	StartedAt time.Time         `json:"startedAt"`
	Config    *TestConfig       `json:"config"`
	Snapshots []MetricsSnapshot `json:"snapshots"`
	Errors    []ErrorEntry      `json:"errors"`
}

type TestState struct {
	mu        sync.RWMutex
	Status    TestStatus
	StartedAt time.Time
	Config    *TestConfig
	Snapshots []MetricsSnapshot
	Errors    []ErrorEntry
}

func NewTestState() *TestState {
	return &TestState{
		Status:    TestStatusIdle,
		Snapshots: make([]MetricsSnapshot, 0),
		Errors:    make([]ErrorEntry, 0),
	}
}

func (s *TestState) SetStatus(status TestStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}

func (s *TestState) GetStatus() TestStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

func (s *TestState) AddSnapshot(snap MetricsSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Snapshots = append(s.Snapshots, snap)
}

func (s *TestState) AddError(entry ErrorEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Errors = append(s.Errors, entry)
	if len(s.Errors) > 1000 {
		s.Errors = s.Errors[len(s.Errors)-500:]
	}
}

func (s *TestState) Reset(cfg *TestConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = TestStatusIdle
	s.Config = cfg
	s.StartedAt = time.Time{}
	s.Snapshots = make([]MetricsSnapshot, 0)
	s.Errors = make([]ErrorEntry, 0)
}

func (s *TestState) GetState() TestStateView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return TestStateView{
		Status:    s.Status,
		StartedAt: s.StartedAt,
		Config:    s.Config,
		Snapshots: s.Snapshots,
		Errors:    s.Errors,
	}
}

type ServiceInfo struct {
	Services []ServiceDesc `json:"services"`
}

type ServiceDesc struct {
	Name    string       `json:"name"`
	Methods []MethodDesc `json:"methods"`
}

type MethodDesc struct {
	Name            string `json:"name"`
	InputType       string `json:"inputType"`
	OutputType      string `json:"outputType"`
	ClientStreaming bool   `json:"clientStreaming"`
	ServerStreaming bool   `json:"serverStreaming"`
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}
