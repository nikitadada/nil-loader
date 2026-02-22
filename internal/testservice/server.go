package testservice

import (
	"context"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	pb "github.com/nikitadada/nil-loader/proto/example"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const degradationThreshold = 800 // RPS после которого начинается деградация

type Server struct {
	pb.UnimplementedExampleServiceServer
	rng        *rand.Rand
	reqCounter atomic.Int64
	currentRPS atomic.Int64
}

func NewServer() *Server {
	s := &Server{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	go s.rpsTracker()
	return s
}

func (s *Server) rpsTracker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		count := s.reqCounter.Swap(0)
		s.currentRPS.Store(count)
	}
}

func (s *Server) ProcessUser(_ context.Context, req *pb.ProcessUserRequest) (*pb.ProcessUserResponse, error) {
	s.reqCounter.Add(1)
	rps := float64(s.currentRPS.Load())

	// Коэффициент перегрузки: 0 при <=800 RPS, растёт экспоненциально выше
	overload := 0.0
	if rps > degradationThreshold {
		overload = (rps - degradationThreshold) / degradationThreshold
	}

	errorRate := baseErrorRate(overload)
	roll := s.rng.Float64() * 100

	if roll < errorRate {
		errType := s.rng.Intn(3)
		switch errType {
		case 0:
			time.Sleep(time.Duration(10+s.rng.Intn(50)) * time.Millisecond)
			return nil, status.Errorf(codes.Unavailable, "service temporarily unavailable for user %s", req.GetUserId())
		case 1:
			time.Sleep(time.Duration(200+s.rng.Intn(500)) * time.Millisecond)
			return nil, status.Errorf(codes.DeadlineExceeded, "processing timed out for %s", req.GetEmail())
		default:
			return nil, status.Errorf(codes.Internal, "internal error processing %s", req.GetUsername())
		}
	}

	latency := baseLatency(s.rng, overload)
	time.Sleep(time.Duration(latency) * time.Millisecond)

	return &pb.ProcessUserResponse{
		Status:      "ok",
		RequestId:   req.GetUserId(),
		ProcessedAt: time.Now().UnixMilli(),
	}, nil
}

// До 800 RPS: ~5% ошибок. При 2x перегрузке: ~40%. При 3x: ~70%.
func baseErrorRate(overload float64) float64 {
	if overload <= 0 {
		return 5.0
	}
	return 5.0 + 35.0*(1-math.Exp(-1.5*overload))
}

// До 800 RPS: 5-50ms. Выше — латентность растёт квадратично.
func baseLatency(rng *rand.Rand, overload float64) int {
	base := 5 + rng.Intn(45)

	if rng.Intn(100) < 8 {
		base += 50 + rng.Intn(100)
	}

	if overload > 0 {
		extra := int(overload * overload * 200)
		base += extra + rng.Intn(1+int(overload*100))
	}

	return base
}
