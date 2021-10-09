package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"

	pb "github.com/artificial-james/import-pipeline/protos/go/resource"
)

type Service struct {
	pb.UnimplementedResourceServiceServer

	redis *redis.Client
}

func NewResource() *Service {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", "redis", 6379),
		// Addr: fmt.Sprintf(":%d", 6379),
	})

	s := new(Service)
	s.redis = rdb

	return s
}

func IncrementID(id string) (string, error) {
	if id == "" || id == "-" {
		return "-", nil
	}

	dashIdx := strings.Index(id, "-")
	if dashIdx > -1 {
		n, err := strconv.Atoi(id[dashIdx+1:])
		if err != nil {
			return "", fmt.Errorf("Cannot increment ID: %v", err)
		}
		return id[:dashIdx+1] + strconv.Itoa(n+1), nil
	}

	n, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("Cannot increment ID: %v", err)
	}
	return strconv.Itoa(n + 1), nil
}

func (s *Service) Create(ctx context.Context, in *pb.CreateResourceRequest) (*pb.Resource, error) {
	if in.Resource == nil || in.Resource.Id == "" {
		in.Resource.Id = fmt.Sprintf("resource_%s", uuid.New().String())
	}

	streamName := fmt.Sprintf("resource.%s", in.Resource.Id)

	rangeN, err := s.redis.XRevRangeN(ctx, streamName, "+", "-", 1).Result()
	if err != nil {
		return nil, err
	}

	nextKey := "0-1"
	if len(rangeN) > 0 {
		lastKey := rangeN[0].ID
		nextKey, err = IncrementID(lastKey)
		if err != nil {
			return nil, err
		}
	}

	protoType := string(in.ProtoReflect().Descriptor().FullName())

	opts := protojson.MarshalOptions{Multiline: false}
	unformattedJson, err := opts.Marshal(in)
	if err != nil {
		return nil, err
	}

	var formattedJson bytes.Buffer
	err = json.Indent(&formattedJson, unformattedJson, "", "  ")
	if err != nil {
		return nil, err
	}

	_, err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		ID:     nextKey,
		Values: map[string]interface{}{protoType: string(formattedJson.Bytes())},
	}).Result()
	if err != nil {
		return nil, err
	}

	return in.Resource, nil
}
