package grpcclient

import (
	"fmt"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/nikitadada/nil-loader/internal/model"
)

func ParseProtoContent(filename, content string) ([]*desc.FileDescriptor, error) {
	parser := protoparse.Parser{
		Accessor: protoparse.FileContentsFromMap(map[string]string{
			filename: content,
		}),
	}

	fds, err := parser.ParseFiles(filename)
	if err != nil {
		return nil, fmt.Errorf("parse proto: %w", err)
	}

	return fds, nil
}

func ListServicesFromProto(fds []*desc.FileDescriptor) *model.ServiceInfo {
	info := &model.ServiceInfo{}

	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			sd := model.ServiceDesc{
				Name:    svc.GetFullyQualifiedName(),
				Methods: make([]model.MethodDesc, 0),
			}
			for _, m := range svc.GetMethods() {
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
	}

	return info
}

func FindMethodDescriptor(fds []*desc.FileDescriptor, serviceName, methodName string) (*desc.MethodDescriptor, error) {
	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			if svc.GetFullyQualifiedName() == serviceName {
				for _, m := range svc.GetMethods() {
					if m.GetName() == methodName {
						return m, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("method %s/%s not found", serviceName, methodName)
}
