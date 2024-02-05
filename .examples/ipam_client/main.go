package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	pbipam "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/env"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// addr = env.Lookup("addr", "localhost:8082")
	addr = env.Lookup("addr", constant.DefaultUnixSocketPath)
)

func main() {
	conn, err := grpc.Dial(fmt.Sprintf("unix:%s", addr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pbipam.NewIpamClient(conn)

	// Contact the server and print out its response.
	// ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()

	succeed := int64(0)
	failed := int64(0)
	wg := sync.WaitGroup{}
	wg.Add(1)
	for i := 0; i < 1; i++ {
		go func(idx int) {
			defer wg.Done()
			r, err := c.Allocate(ctx, &pbipam.AllocateRequest{
				ContainerID:   "fake",
				IfName:        fmt.Sprintf("eth0-%v", idx),
				NetNamespace:  "fake",
				DefaultSubnet: "subnet",
				PodName:       "samplepod",
				PodNamespace:  "default",
				PodUID:        "fake",
			})
			if err != nil {
				atomic.AddInt64(&failed, 1)
				fmt.Printf("could not allocate: %v\n", err)
				return
				// log.Fatalf("could not allocate: %v", err)
			}
			atomic.AddInt64(&succeed, 1)
			log.Printf("%v\tallocate: %+v\n", idx, r)
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(start).Seconds()
	fmt.Printf("方法执行耗时: %.2f 秒\n", elapsed)
	fmt.Printf("成功: %v\t失败: %v\n", succeed, failed)
}
