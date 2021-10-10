package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	pb "github.com/artificial-james/import-pipeline/protos/go/import"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(bufio.ScanBytes)
	importBuffer := make([]byte, 0, 1024*1024)

	for scanner.Scan() {
		importBuffer = append(importBuffer, scanner.Bytes()[0])
	}

	importProto := new(pb.ResourcePayload)
	err := protojson.Unmarshal(importBuffer, importProto)
	if err != nil {
		panic(err)
	}

	client, closeFn := Connect()
	defer closeFn()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := client.ImportTransaction(ctx, &pb.ImportRequest{ProtoPayload: importProto})
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(os.Stderr, "Imported [%d] resources...", res.Operations)
}

func Connect() (pb.ImportServiceClient, func()) {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	// conn, err := grpc.Dial(fmt.Sprintf("%s:%d", "import-service", 8000), opts...)
	conn, err := grpc.Dial(fmt.Sprintf(":%d", 8000), opts...)
	if err != nil {
		panic(err)
	}

	return pb.NewImportServiceClient(conn), func() {
		conn.Close()
	}
}
