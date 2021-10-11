package imp

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/artificial-james/import-pipeline/protos/go/import"
	resourcepb "github.com/artificial-james/import-pipeline/protos/go/resource"
)

var numRunners = uint(runtime.NumCPU())

type Service struct {
	pb.UnimplementedImportServiceServer

	client resourcepb.ResourceServiceClient
	redis  *redis.Client
}

func NewImport(ctx context.Context) (*Service, <-chan struct{}) {
	done := make(chan struct{})

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", "redis", 6379),
		// Addr: fmt.Sprintf(":%d", 6379),
	})

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
	s.redis = rdb

	return s, done
}

func (s *Service) Import(ctx context.Context, in *pb.ImportRequest) (*pb.ImportResponse, error) {
	fmt.Println("--> Import")

	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	for _, resource := range in.ProtoPayload.Resources {
		req := &resourcepb.CreateResourceRequest{
			Resource: resource,
		}
		_, err := s.client.Create(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	return &pb.ImportResponse{Operations: uint32(len(in.ProtoPayload.Resources))}, nil
}

func IncrementID(id string) (string, error) {
	if id == "" || id == "-" {
		return "-", nil
	}

	dashIdx := strings.Index(id, "-")
	if dashIdx > -1 {
		n, err := strconv.Atoi(id[dashIdx+1:])
		if err != nil {
			return "", fmt.Errorf("cannot increment ID: %v", err)
		}
		return id[:dashIdx+1] + strconv.Itoa(n+1), nil
	}

	n, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("Cannot increment ID: %v", err)
	}
	return strconv.Itoa(n + 1), nil
}

func (s *Service) asyncQueue(ctx context.Context, resources []*resourcepb.Resource) <-chan interface{} {
	out := make(chan interface{}, numRunners)
	go func() {
		defer close(out)
		for _, resource := range resources {
			req := &resourcepb.CreateResourceRequest{
				Resource: resource,
			}
			r, err := s.client.QueueImport(ctx, req)
			if err != nil {
				out <- err
				break
			}

			for _, q := range r.Queue {
				out <- q
			}
		}
	}()
	return out
}

type Info struct {
	StreamName string
	ID         string
}

func (s *Service) asyncInfos(
	ctx context.Context,
	tx *redis.Tx,
	imports []*resourcepb.ImportQueue,
) <-chan interface{} {
	out := make(chan interface{}, numRunners)
	go func() {
		defer close(out)
		for _, q := range imports {
			if q.StreamId != "" || q.ForceId {
				info, err := tx.XInfoStream(ctx, q.StreamName).Result()
				if err != nil && err.Error() == "ERR no such key" {
					info = &redis.XInfoStream{
						LastGeneratedID: "0-0",
					}
				} else if err != nil {
					out <- err
					break
				}

				if q.StreamId != "" && q.StreamId != info.LastGeneratedID {
					out <- fmt.Errorf("stream modified since import request started")
					break
				}

				if q.ForceId {
					nextID, err := IncrementID(info.LastGeneratedID)
					if err != nil {
						out <- err
						break
					}

					out <- Info{q.StreamName, nextID}
				}
			}
		}
	}()
	return out
}

func (s *Service) asyncAdds(
	ctx context.Context,
	pipe redis.Pipeliner,
	imports []*resourcepb.ImportQueue,
) <-chan error {
	out := make(chan error, numRunners)
	go func() {
		defer close(out)
		for _, q := range imports {
			err := pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: q.StreamName,
				ID:     q.StreamId,
				Values: q.Values,
			}).Err()

			if err != nil {
				out <- err
				break
			}
		}
	}()
	return out
}

func (s *Service) ImportTransaction(ctx context.Context, in *pb.ImportRequest) (*pb.ImportResponse, error) {
	fmt.Println("--> ImportTransaction")

	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	queueChan := s.asyncQueue(ctx, in.ProtoPayload.Resources)
	queue := make([]*resourcepb.ImportQueue, 0, len(in.ProtoPayload.Resources))
	for q := range queueChan {
		// for q := range sink.Out {
		if task, ok := q.(*resourcepb.ImportQueue); ok {
			queue = append(queue, task)
		} else if err, ok := q.(error); ok {
			return nil, err
		}
	}

	streams := make(map[string]*resourcepb.ImportQueue)
	streamNames := make([]string, 0, len(queue))
	imports := make([]*resourcepb.ImportQueue, 0, len(queue))
	for _, q := range queue {
		if streams[q.StreamName] != nil {
			return nil, fmt.Errorf("cannot update the same stream name more than once [%s]", q.StreamName)
		}
		streams[q.StreamName] = q
		streamNames = append(streamNames, q.StreamName)
		imports = append(imports, q)
	}

	err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
		infoChan := s.asyncInfos(ctx, tx, imports)
		for i := range infoChan {
			if info, ok := i.(Info); ok {
				streams[info.StreamName].StreamId = info.ID
			} else if err, ok := i.(error); ok {
				return err
			}
		}

		// TODO:  Remove me when done with demos.
		time.Sleep(5 * time.Second)

		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			addChan := s.asyncAdds(ctx, pipe, imports)
			for a := range addChan {
				if err, ok := a.(error); ok {
					return err
				}
			}
			return nil
		})
		return err
	}, streamNames...)

	if err != nil {
		return nil, err
	}

	return &pb.ImportResponse{Operations: uint32(len(streamNames))}, nil
}
