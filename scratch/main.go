package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/go-redis/redis/v8"
)

const maxRetries = 1000

func main() {
	client := redis.NewClient(&redis.Options{})

	// cmd := client.XAdd(context.Background(), &redis.XAddArgs{
	// 	Stream: "james-stream",
	// 	ID:     "0-1",
	// 	Values: map[string]interface{}{"a": 1, "b": 2},
	// })
	//
	// fmt.Println(cmd.String())

	ctx := context.Background()
	// Increment transactionally increments key using GET and SET commands.
	increment := func(key string) error {
		// Transactional function.
		txf := func(tx *redis.Tx) error {
			// Get current value or zero.
			n, err := tx.Get(ctx, key).Int()
			if err != nil && err != redis.Nil {
				return err
			}

			// Actual opperation (local in optimistic lock).
			n++

			// Operation is committed only if the watched keys remain unchanged.
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, n, 0)
				return nil
			})
			return err
		}

		for i := 0; i < maxRetries; i++ {
			err := client.Watch(ctx, txf, key)
			if err == nil {
				// Success.
				fmt.Println("succeeded after tries: ", i)
				return nil
			}
			if err == redis.TxFailedErr {
				// Optimistic lock lost. Retry.
				continue
			}
			// Return any other error.
			return err
		}

		return errors.New("increment reached maximum number of retries")
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := increment("counter3"); err != nil {
				fmt.Println("increment error:", err)
			}
		}()
	}
	wg.Wait()

	n, err := client.Get(ctx, "counter3").Int()
	fmt.Println("ended with", n, err)
}
