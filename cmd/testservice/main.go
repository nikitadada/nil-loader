package main

import (
	"log"
	"net"

	"github.com/nikitadada/nil-loader/internal/testservice"
	ordersv1 "github.com/nikitadada/nil-loader/proto/demo/orders/v1"
	paymentsv1 "github.com/nikitadada/nil-loader/proto/demo/payments/v1"
	userv1 "github.com/nikitadada/nil-loader/proto/demo/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	handler := testservice.NewServer()
	userv1.RegisterUserServiceServer(srv, handler)
	ordersv1.RegisterOrdersServiceServer(srv, handler)
	paymentsv1.RegisterPaymentsServiceServer(srv, handler)
	reflection.Register(srv)

	log.Println("Test gRPC service started on :50051 (with reflection)")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
