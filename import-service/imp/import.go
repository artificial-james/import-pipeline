package imp

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/artificial-james/import-pipeline/protos/go/import"
	resourcepb "github.com/artificial-james/import-pipeline/protos/go/resource"
)

type Service struct {
	pb.UnimplementedImportServiceServer

	client resourcepb.ResourceServiceClient
}

func NewImport(ctx context.Context) (*Service, <-chan struct{}) {
	done := make(chan struct{})

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	host := fmt.Sprintf("%s:%d", "resource-service", 8001)
	// host := fmt.Sprintf(":%d", 8001)
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithBlock()}

	conn, err := grpc.DialContext(ctxWithTimeout, host, opts...)
	if err != nil {
		panic("cannot connect to client")
	}

	go func() {
		<-ctx.Done()
		cancel()
		err := conn.Close()
		if err != nil {
			fmt.Printf("cannot close client connection\n")
		}
		close(done)
	}()

	s := new(Service)
	s.client = resourcepb.NewResourceServiceClient(conn)

	return s, done
}

func (s *Service) Import(ctx context.Context, in *pb.ImportRequest) (*pb.ImportResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	ids := make([]string, len(in.ProtoPayload.Resources))
	for idx, resource := range in.ProtoPayload.Resources {
		req := &resourcepb.CreateResourceRequest{
			Resource: resource,
		}
		r, err := s.client.Create(ctx, req)
		if err != nil {
			return nil, err
		}

		ids[idx] = r.Id
	}

	return &pb.ImportResponse{Ids: ids}, nil
}
