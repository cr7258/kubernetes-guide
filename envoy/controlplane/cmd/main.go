package main

import (
	"context"
	"fmt"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"log"
	"myistio/controlplane/utils"
	"net"
	"os"
	"time"
)

func main() {
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions,
		grpc.MaxConcurrentStreams(1000), //一条GRPC连接允许并发的发送和接收多个Stream
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    time.Second * 30, //连接超过多少时间 不活跃，则会去探测 是否依然alive
			Timeout: time.Second * 5,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Second * 30, //发送ping之前最少要等待 时间
			PermitWithoutStream: true,             //连接空闲时仍然发送PING帧监测
		}),
	)
	//创建grpc 服务
	grpcServer := grpc.NewServer(grpcOptions...)

	//日志
	llog := utils.MyLogger{}
	//创建缓存系统
	configCache := cache.NewSnapshotCache(false, cache.IDHash{}, llog)

	snapshot := utils.GenerateSnapshot("v1") //envoy 配置的 缓存快照 ---内容是写死
	if err := snapshot.Consistent(); err != nil {
		llog.Errorf("snapshot inconsistency: %+v\n%+v", snapshot, err)
		os.Exit(1)
	}

	// nodeID 必须要设置
	nodeID := "test1"
	if err := configCache.SetSnapshot(context.Background(), nodeID, snapshot); err != nil {
		os.Exit(1)
	}

	// 请求回调
	cb := &utils.Callbacks{Debug: llog.Debug}

	// 官方提供的控制面 server
	srv := server.NewServer(context.Background(), configCache, cb)
	//注册 Cluster
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, srv)
	//注册 Listener
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, srv)
	// 注册 Route
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, srv)

	errCh := make(chan error)
	go func() {
		// 我们的grpc 服务是19000
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 19090))
		if err != nil {
			errCh <- err
			return
		}
		//启动grpc 服务
		if err = grpcServer.Serve(lis); err != nil {
			errCh <- err
			return
		}
	}()
	go func() {
		r := gin.New()
		r.GET("/test", func(c *gin.Context) {
			utils.UpstreamHost = "10.88.0.3"
			ss := utils.GenerateSnapshot("v2")
			err := configCache.SetSnapshot(c, nodeID, ss)
			if err != nil {
				c.String(400, err.Error())
			} else {
				c.String(200, "Ok")
			}
		})
		err := r.Run(":18000")
		if err != nil {
			errCh <- err
		}

	}()
	err := <-errCh
	log.Println(err)
	os.Exit(1)
}
