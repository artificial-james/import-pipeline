package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"google.golang.org/grpc"

	"github.com/artificial-james/import-pipeline/import-service/imp"
	pb "github.com/artificial-james/import-pipeline/protos/go/import"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcServer := grpc.NewServer()
	importService, done := imp.NewImport(ctx)
	pb.RegisterImportServiceServer(grpcServer, importService)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 8000))
	if err != nil {
		panic(err)
	}
	err = grpcServer.Serve(lis)
	if err != nil {
		panic(err)
	}

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		s := <-sigC
		fmt.Printf("Got signal %v, attempting graceful shutdown\n", s)

		cancel()
		grpcServer.GracefulStop()

		// Make sure all clients close before killing the server
		<-done
		wg.Done()
	}()

	wg.Wait()
	fmt.Printf("Shut down complete\n")
}
