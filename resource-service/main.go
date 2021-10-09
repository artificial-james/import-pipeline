package main

import (
	"fmt"
	"net"

	"google.golang.org/grpc"

	pb "github.com/artificial-james/import-pipeline/protos/go/resource"
	"github.com/artificial-james/import-pipeline/resource-service/resource"
)

func main() {
	grpcServer := grpc.NewServer()
	resourceService := resource.NewResource()
	pb.RegisterResourceServiceServer(grpcServer, resourceService)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 8001))
	if err != nil {
		panic(err)
	}
	err = grpcServer.Serve(lis)
	if err != nil {
		panic(err)
	}
}
