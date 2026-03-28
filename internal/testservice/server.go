package testservice

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	ordersv1 "github.com/nikitadada/nil-loader/proto/demo/orders/v1"
	paymentsv1 "github.com/nikitadada/nil-loader/proto/demo/payments/v1"
	userv1 "github.com/nikitadada/nil-loader/proto/demo/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const degradationThreshold = 800 // RPS после которого начинается деградация

type Server struct {
	userv1.UnimplementedUserServiceServer
	ordersv1.UnimplementedOrdersServiceServer
	paymentsv1.UnimplementedPaymentsServiceServer

	rngMu      sync.Mutex
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

func (s *Server) overload() float64 {
	s.reqCounter.Add(1)
	rps := float64(s.currentRPS.Load())

	// Коэффициент перегрузки: 0 при <=800 RPS, растёт экспоненциально выше
	overload := 0.0
	if rps > degradationThreshold {
		overload = (rps - degradationThreshold) / degradationThreshold
	}
	return overload
}

func (s *Server) ProcessUser(_ context.Context, req *userv1.ProcessUserRequest) (*userv1.ProcessUserResponse, error) {
	overload := s.overload()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "service temporarily unavailable for user %s", req.GetUserId())
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "processing timed out for %s", req.GetEmail())
		default:
			return status.Errorf(codes.Internal, "internal error processing %s", req.GetUsername())
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	return &userv1.ProcessUserResponse{
		Status:      "ok",
		RequestId:   req.GetUserId(),
		ProcessedAt: time.Now().UnixMilli(),
	}, nil
}

func (s *Server) CreateOrder(_ context.Context, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	overload := s.overload()

	order := req.GetOrder()
	requestID := ""
	if order != nil {
		requestID = order.GetOrderId()
	}

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "orders temporarily unavailable for order %s", requestID)
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "create order timed out for user %s", safeStr(order.GetUserId()))
		default:
			return status.Errorf(codes.Internal, "internal error creating order %s", requestID)
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	respOrder := order
	if respOrder == nil {
		respOrder = &ordersv1.Order{
			OrderId:     requestID,
			Status:      ordersv1.OrderStatus_ORDER_STATUS_CREATED,
			CreatedAtMs: time.Now().UnixMilli(),
			UserId:      "",
			Items:       nil,
			Total:       nil,
		}
	} else if respOrder.GetCreatedAtMs() == 0 {
		respOrder.CreatedAtMs = time.Now().UnixMilli()
	}
	if respOrder.GetStatus() == ordersv1.OrderStatus_ORDER_STATUS_UNKNOWN {
		respOrder.Status = ordersv1.OrderStatus_ORDER_STATUS_CREATED
	}

	return &ordersv1.CreateOrderResponse{
		Status:    "ok",
		RequestId: requestID,
		Order:     respOrder,
	}, nil
}

func (s *Server) GetOrder(_ context.Context, req *ordersv1.GetOrderRequest) (*ordersv1.GetOrderResponse, error) {
	overload := s.overload()
	orderID := req.GetOrderId()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "orders temporarily unavailable for order %s", orderID)
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "get order timed out for %s", orderID)
		default:
			return status.Errorf(codes.Internal, "internal error getting order %s", orderID)
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	order := &ordersv1.Order{
		OrderId:     orderID,
		UserId:      "",
		Status:      ordersv1.OrderStatus_ORDER_STATUS_PAID,
		CreatedAtMs: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if req.GetIncludeItems() {
		order.Items = []*ordersv1.OrderItem{
			{Sku: "sku_demo_1", Quantity: 1, Price: &ordersv1.Money{Units: 10, Nanos: 0, CurrencyCode: "RUB"}},
			{Sku: "sku_demo_2", Quantity: 2, Price: &ordersv1.Money{Units: 5, Nanos: 0, CurrencyCode: "RUB"}},
		}
		order.Total = &ordersv1.Money{Units: 20, Nanos: 0, CurrencyCode: "RUB"}
	}

	return &ordersv1.GetOrderResponse{
		Status:    "ok",
		RequestId: orderID,
		Order:     order,
	}, nil
}

func (s *Server) ListOrders(_ context.Context, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	overload := s.overload()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "orders temporarily unavailable for user %s", req.GetUserId())
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "list orders timed out for user %s", req.GetUserId())
		default:
			return status.Errorf(codes.Internal, "internal error listing orders for user %s", req.GetUserId())
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	now := time.Now().UnixMilli()
	orders := []*ordersv1.Order{
		{
			OrderId:     "order_demo_1",
			UserId:      req.GetUserId(),
			Status:      chooseOrderStatus(req.GetStatusFilter()),
			CreatedAtMs: now - 10_000,
			Items: []*ordersv1.OrderItem{
				{Sku: "sku_demo_1", Quantity: 1, Price: &ordersv1.Money{Units: 10, CurrencyCode: "RUB"}},
			},
			Total: &ordersv1.Money{Units: 10, CurrencyCode: "RUB"},
		},
		{
			OrderId:     "order_demo_2",
			UserId:      req.GetUserId(),
			Status:      chooseOrderStatus(req.GetStatusFilter()),
			CreatedAtMs: now - 20_000,
			Items: []*ordersv1.OrderItem{
				{Sku: "sku_demo_2", Quantity: 2, Price: &ordersv1.Money{Units: 5, CurrencyCode: "RUB"}},
			},
			Total: &ordersv1.Money{Units: 10, CurrencyCode: "RUB"},
		},
	}

	return &ordersv1.ListOrdersResponse{
		Status:        "ok",
		RequestId:     req.GetUserId(),
		Orders:        orders,
		NextPageToken: "",
	}, nil
}

func (s *Server) Authorize(_ context.Context, req *paymentsv1.AuthorizeRequest) (*paymentsv1.AuthorizeResponse, error) {
	overload := s.overload()
	paymentID := req.GetPaymentId()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "payments temporarily unavailable for payment %s", paymentID)
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "authorize timed out for order %s", req.GetOrderId())
		default:
			return status.Errorf(codes.Internal, "internal error authorizing payment %s", paymentID)
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	authCode := s.authCode(paymentID)
	return &paymentsv1.AuthorizeResponse{
		Status:     "ok",
		RequestId:  paymentID,
		Authorized: true,
		AuthCode:   authCode,
	}, nil
}

func (s *Server) Capture(_ context.Context, req *paymentsv1.CaptureRequest) (*paymentsv1.CaptureResponse, error) {
	overload := s.overload()
	paymentID := req.GetPaymentId()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "payments temporarily unavailable for payment %s", paymentID)
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "capture timed out for payment %s", paymentID)
		default:
			return status.Errorf(codes.Internal, "internal error capturing payment %s", paymentID)
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	return &paymentsv1.CaptureResponse{
		Status:    "ok",
		RequestId: paymentID,
		Captured:  true,
	}, nil
}

func (s *Server) Refund(_ context.Context, req *paymentsv1.RefundRequest) (*paymentsv1.RefundResponse, error) {
	overload := s.overload()
	paymentID := req.GetPaymentId()

	if err := s.maybeFail(overload, func(p failPlan) error {
		switch p.errType {
		case 0:
			time.Sleep(time.Duration(10+(p.b%50)) * time.Millisecond)
			return status.Errorf(codes.Unavailable, "payments temporarily unavailable for payment %s", paymentID)
		case 1:
			time.Sleep(time.Duration(200+(p.a%500)) * time.Millisecond)
			return status.Errorf(codes.DeadlineExceeded, "refund timed out for payment %s", paymentID)
		default:
			return status.Errorf(codes.Internal, "internal error refunding payment %s", paymentID)
		}
	}); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(s.latencyMs(overload)) * time.Millisecond)

	return &paymentsv1.RefundResponse{
		Status:    "ok",
		RequestId: paymentID,
		Refunded:  true,
		RefundId:  "refund_" + paymentID,
	}, nil
}

type failPlan struct {
	errType int
	a       int
	b       int
}

func (s *Server) maybeFail(overload float64, buildErr func(p failPlan) error) error {
	errorRate := baseErrorRate(overload)

	s.rngMu.Lock()
	roll := s.rng.Float64() * 100
	if roll >= errorRate {
		s.rngMu.Unlock()
		return nil
	}
	p := failPlan{
		errType: s.rng.Intn(3),
		a:       s.rng.Intn(1000),
		b:       s.rng.Intn(1000),
	}
	s.rngMu.Unlock()

	return buildErr(p)
}

func (s *Server) latencyMs(overload float64) int {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	return baseLatency(s.rng, overload)
}

func (s *Server) authCode(seed string) string {
	// Детерминированный-ish код для демо, но с небольшим рандомом.
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	return seed + "_" + "AC" + itoa2(s.rng.Intn(100))
}

func chooseOrderStatus(filter ordersv1.OrderStatus) ordersv1.OrderStatus {
	if filter != ordersv1.OrderStatus_ORDER_STATUS_UNKNOWN {
		return filter
	}
	return ordersv1.OrderStatus_ORDER_STATUS_PAID
}

func safeStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func itoa2(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	// n expected < 100 in our use.
	tens := n / 10
	ones := n % 10
	return string(rune('0'+tens)) + string(rune('0'+ones))
}

// До 800 RPS: ~3% ошибок.
// При 1600 RPS (~(1600-800)/800=1 перегрузка): ~30%.
// При 2400 RPS (~2 перегрузки): ~35%. Асимптота: ~37%.
func baseErrorRate(overload float64) float64 {
	if overload <= 0 {
		return 3.0
	}
	return 3.0 + 34.0*(1-math.Exp(-1.5*overload))
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
