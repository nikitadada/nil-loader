package grpcclient

import (
	"context"
	"fmt"
	"sort"

	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/nikitadada/nil-loader/internal/model"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func ListServicesViaReflection(ctx context.Context, conn *grpc.ClientConn) (*model.ServiceInfo, error) {
	client := grpcreflect.NewClientV1Alpha(ctx, rpb.NewServerReflectionClient(conn))
	defer client.Reset()

	serviceNames, err := client.ListServices()
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	sort.Strings(serviceNames)

	info := &model.ServiceInfo{}
	for _, svcName := range serviceNames {
		if svcName == "grpc.reflection.v1alpha.ServerReflection" || svcName == "grpc.reflection.v1.ServerReflection" {
			continue
		}

		svcDesc, err := client.ResolveService(svcName)
		if err != nil {
			continue
		}

		sd := model.ServiceDesc{
			Name:    svcName,
			Methods: make([]model.MethodDesc, 0),
		}

		for _, m := range svcDesc.GetMethods() {
			sd.Methods = append(sd.Methods, model.MethodDesc{
				Name:            m.GetName(),
				InputType:       m.GetInputType().GetFullyQualifiedName(),
				OutputType:      m.GetOutputType().GetFullyQualifiedName(),
				ClientStreaming: m.IsClientStreaming(),
				ServerStreaming: m.IsServerStreaming(),
			})
		}
		info.Services = append(info.Services, sd)
	}

	return info, nil
}
