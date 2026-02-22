package main

import (
	"log"
	"net"

	"github.com/nikitadada/nil-loader/internal/testservice"
	pb "github.com/nikitadada/nil-loader/proto/example"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterExampleServiceServer(srv, testservice.NewServer())
	reflection.Register(srv)

	log.Println("Test gRPC service started on :50051 (with reflection)")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
