package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type Caller struct {
	conn       *grpc.ClientConn
	stub       grpcdynamic.Stub
	methodDesc *desc.MethodDescriptor
}

func NewCaller(target string, useReflection bool, serviceName, methodName string, protoContent string) (*Caller, error) {
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(50*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}

	var md *desc.MethodDescriptor

	if useReflection {
		md, err = resolveViaReflection(conn, serviceName, methodName)
		if err != nil {
			conn.Close()
			return nil, err
		}
	} else {
		fds, err := ParseProtoContent("input.proto", protoContent)
		if err != nil {
			conn.Close()
			return nil, err
		}
		md, err = FindMethodDescriptor(fds, serviceName, methodName)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	return &Caller{
		conn:       conn,
		stub:       grpcdynamic.NewStub(conn),
		methodDesc: md,
	}, nil
}

func (c *Caller) Call(ctx context.Context, payload []byte) (time.Duration, error) {
	msg := dynamic.NewMessage(c.methodDesc.GetInputType())
	if len(payload) > 0 {
		if err := msg.UnmarshalJSON(payload); err != nil {
			return 0, fmt.Errorf("unmarshal payload: %w", err)
		}
	}

	start := time.Now()
	resp, err := c.stub.InvokeRpc(ctx, c.methodDesc, msg)
	elapsed := time.Since(start)

	if err != nil {
		return elapsed, err
	}

	_ = resp
	return elapsed, nil
}

func (c *Caller) CallAndReturn(ctx context.Context, payload []byte) (time.Duration, json.RawMessage, error) {
	msg := dynamic.NewMessage(c.methodDesc.GetInputType())
	if len(payload) > 0 {
		if err := msg.UnmarshalJSON(payload); err != nil {
			return 0, nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}

	start := time.Now()
	resp, err := c.stub.InvokeRpc(ctx, c.methodDesc, msg)
	elapsed := time.Since(start)

	if err != nil {
		return elapsed, nil, err
	}

	dynResp, ok := resp.(*dynamic.Message)
	if !ok {
		return elapsed, nil, nil
	}

	respJSON, err := dynResp.MarshalJSON()
	if err != nil {
		return elapsed, nil, nil
	}

	return elapsed, respJSON, nil
}

func (c *Caller) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func resolveViaReflection(conn *grpc.ClientConn, serviceName, methodName string) (*desc.MethodDescriptor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := grpcreflect.NewClientV1Alpha(ctx, rpb.NewServerReflectionClient(conn))
	defer client.Reset()

	svcDesc, err := client.ResolveService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("resolve service %s: %w", serviceName, err)
	}

	md := svcDesc.FindMethodByName(methodName)
	if md == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	return md, nil
}
