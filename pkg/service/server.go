// Copyright 2023 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/pion/turn/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/twitchtv/twirp"
	"github.com/urfave/negroni/v3"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils/xtwirp"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/version"
)

// LivekitServer 定义了LivekitServer结构体
type LivekitServer struct {
	// 核心服务组件
	roomManager  *RoomManager  // 管理房间的生命周期、参与者和媒体会话
	signalServer *SignalServer // 处理WebRTC连接建立的信令交换和会话协商
	turnServer   *turn.Server  // 提供NAT穿透服务,确保在复杂网络环境下的连接性
	// 媒体服务
	rtcService   *RTCService    // 处理WebRTC连接、媒体流传输和SFU功能
	ioService    *IOInfoService // 处理媒体流的输入输出统计和监控
	agentService *AgentService  // 处理媒体处理代理(如录制、转码等)的管理
	// 网络和API服务
	httpServer *http.Server   // 提供REST API和WebSocket接入点
	promServer *http.Server   // 提供Prometheus指标采集接口
	router     routing.Router // 处理分布式节点间的消息路由和负载均衡
	// 节点状态管理
	config      *config.Config    // 服务器全局配置,包含所有服务的配置参数
	currentNode routing.LocalNode // 当前节点在集群中的标识和状态信息
	running     atomic.Bool       // 服务器运行状态标志
	doneChan    chan struct{}     // 服务关闭信号通道
	closedChan  chan struct{}     // 服务完全停止信号通道
}

// NewLivekitServer 创建LivekitServer实例
// NewLivekitServer creates a LivekitServer instance
func NewLivekitServer(
	conf *config.Config,
	roomService livekit.RoomService,
	agentDispatchService *AgentDispatchService,
	egressService *EgressService,
	ingressService *IngressService,
	sipService *SIPService,
	ioService *IOInfoService,
	rtcService *RTCService,
	agentService *AgentService,
	keyProvider auth.KeyProvider,
	router routing.Router,
	roomManager *RoomManager,
	signalServer *SignalServer,
	turnServer *turn.Server,
	currentNode routing.LocalNode,
) (s *LivekitServer, err error) {
	s = &LivekitServer{
		config:       conf,
		ioService:    ioService,
		rtcService:   rtcService,
		agentService: agentService,
		router:       router,
		roomManager:  roomManager,
		signalServer: signalServer,
		// turn server starts automatically at NewTurnServer
		// 在NewTurnServer中自动启动, 监听已经启动了
		turnServer:  turnServer,
		currentNode: currentNode,
		closedChan:  make(chan struct{}),
	}

	// 中间件管理
	// 	错误恢复
	// CORS处理
	// URL清理
	// API认证
	middlewares := []negroni.Handler{
		// always first // 1. 错误恢复中间件
		negroni.NewRecovery(),
		// CORS is allowed, we rely on token authentication to prevent improper use
		cors.New(cors.Options{
			AllowOriginFunc: func(origin string) bool {
				return true // 允许所有来源
			},
			AllowedHeaders: []string{"*"}, // 允许所有请求头
			// allow preflight to be cached for a day
			MaxAge: 86400, // preflight 请求缓存时间为1天
		}),
		negroni.HandlerFunc(RemoveDoubleSlashes), // 移除 URL 中的重复斜杠
	}
	if keyProvider != nil {
		middlewares = append(middlewares, NewAPIKeyAuthMiddleware(keyProvider))
	}

	serverOptions := []interface{}{
		twirp.WithServerHooks(twirp.ChainHooks(
			TwirpLogger(),
			TwirpRequestStatusReporter(),
		)),
	}
	for _, opt := range xtwirp.DefaultServerOptions() {
		serverOptions = append(serverOptions, opt)
	}
	roomServer := livekit.NewRoomServiceServer(roomService, serverOptions...)
	agentDispatchServer := livekit.NewAgentDispatchServiceServer(agentDispatchService, serverOptions...)
	egressServer := livekit.NewEgressServer(egressService, serverOptions...)
	ingressServer := livekit.NewIngressServer(ingressService, serverOptions...)
	sipServer := livekit.NewSIPServer(sipService, serverOptions...)

	mux := http.NewServeMux()
	if conf.Development {
		// pprof handlers are registered onto DefaultServeMux
		// 将pprof处理程序注册到DefaultServeMux
		mux = http.DefaultServeMux
		mux.HandleFunc("/debug/goroutine", s.debugGoroutines)
		mux.HandleFunc("/debug/rooms", s.debugInfo)
	}

	xtwirp.RegisterServer(mux, roomServer)          // 注册房间服务
	xtwirp.RegisterServer(mux, agentDispatchServer) // 注册代理调度服务
	xtwirp.RegisterServer(mux, egressServer)        // 注册出口服务
	xtwirp.RegisterServer(mux, ingressServer)       // 注册入口服务
	xtwirp.RegisterServer(mux, sipServer)           // 注册SIP服务

	mux.Handle("/rtc", rtcService) // 处理WebRTC连接
	rtcService.SetupRoutes(mux)    // 设置WebRTC连接的路由

	mux.Handle("/agent", agentService)    // 处理代理服务
	mux.HandleFunc("/", s.defaultHandler) // 处理默认请求

	s.httpServer = &http.Server{
		Handler: configureMiddlewares(mux, middlewares...),
	}

	// 启动Prometheus服务
	if conf.Prometheus.Port > 0 {
		promHandler := promhttp.Handler() // 创建一个prometheus指标采集的handler
		if conf.Prometheus.Username != "" && conf.Prometheus.Password != "" {
			protectedHandler := negroni.New()
			protectedHandler.Use(negroni.HandlerFunc(GenBasicAuthMiddleware(conf.Prometheus.Username, conf.Prometheus.Password)))
			protectedHandler.UseHandler(promHandler)
			promHandler = protectedHandler
		}
		s.promServer = &http.Server{
			Handler: promHandler,
		}
	}

	if err = router.RemoveDeadNodes(); err != nil {
		return
	}

	return
}

func (s *LivekitServer) Node() *livekit.Node {
	return s.currentNode.Clone()
}

func (s *LivekitServer) HTTPPort() int {
	return int(s.config.Port)
}

func (s *LivekitServer) IsRunning() bool {
	return s.running.Load()
}

func (s *LivekitServer) Start() error {
	if s.running.Load() {
		return errors.New("already running")
	}
	s.doneChan = make(chan struct{})

	// 注册/注销节点（退出时）
	if err := s.router.RegisterNode(); err != nil {
		return err
	}
	defer func() {
		if err := s.router.UnregisterNode(); err != nil {
			logger.Errorw("could not unregister node", err)
		}
	}()

	// 启动路由（维持节点的最新状态与指标到存储中心）
	// 启动的两个协程会定时更新节点状态与指标，类似心跳的机制
	if err := s.router.Start(); err != nil {
		return err
	}

	// 启动IO服务
	if err := s.ioService.Start(); err != nil {
		return err
	}

	addresses := s.config.BindAddresses
	if addresses == nil {
		addresses = []string{""}
	}

	// ensure we could listen
	listeners := make([]net.Listener, 0)
	promListeners := make([]net.Listener, 0)
	for _, addr := range addresses {
		// 侦听HTTP服务
		ln, err := net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(int(s.config.Port))))
		if err != nil {
			return err
		}
		listeners = append(listeners, ln)

		// 侦听Prometheus服务
		if s.promServer != nil {
			ln, err = net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(int(s.config.Prometheus.Port))))
			if err != nil {
				return err
			}
			promListeners = append(promListeners, ln)
		}
	}

	values := []interface{}{
		"portHttp", s.config.Port,
		"nodeID", s.currentNode.NodeID(),
		"nodeIP", s.currentNode.NodeIP(),
		"version", version.Version,
	}
	if s.config.BindAddresses != nil {
		values = append(values, "bindAddresses", s.config.BindAddresses)
	}
	if s.config.RTC.TCPPort != 0 {
		values = append(values, "rtc.portTCP", s.config.RTC.TCPPort)
	}
	if !s.config.RTC.ForceTCP && s.config.RTC.UDPPort.Valid() {
		values = append(values, "rtc.portUDP", s.config.RTC.UDPPort)
	} else {
		values = append(values,
			"rtc.portICERange", []uint32{s.config.RTC.ICEPortRangeStart, s.config.RTC.ICEPortRangeEnd},
		)
	}
	if s.config.Prometheus.Port != 0 {
		values = append(values, "portPrometheus", s.config.Prometheus.Port)
	}
	if s.config.Region != "" {
		values = append(values, "region", s.config.Region)
	}
	logger.Infow("starting LiveKit server", values...)
	if runtime.GOOS == "windows" {
		logger.Infow("Windows detected, capacity management is unavailable")
	}

	// 启动Prometheus服务
	for _, promLn := range promListeners {
		go s.promServer.Serve(promLn)
	}

	// 启动信号服务
	if err := s.signalServer.Start(); err != nil {
		return err
	}

	httpGroup := &errgroup.Group{}

	// 启动HTTP服务
	for _, ln := range listeners {
		l := ln
		httpGroup.Go(func() error {
			return s.httpServer.Serve(l)
		})
	}
	go func() {
		if err := httpGroup.Wait(); err != http.ErrServerClosed {
			logger.Errorw("could not start server", err)
			s.Stop(true)
		}
	}()

	// 定时服务(1秒)执行关闭空闲房间
	go s.backgroundWorker()

	// give time for Serve goroutine to start
	time.Sleep(100 * time.Millisecond)

	s.running.Store(true)

	<-s.doneChan

	// wait for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_ = s.httpServer.Shutdown(ctx)

	if s.promServer != nil {
		_ = s.promServer.Shutdown(ctx)
	}

	if s.turnServer != nil {
		_ = s.turnServer.Close()
	}

	s.roomManager.Stop()
	s.signalServer.Stop()
	s.ioService.Stop()

	close(s.closedChan)
	return nil
}

// Stop 停止LivekitServer
// 在main.go中调用, 第一次是优雅关闭, 第二次是强制关闭
func (s *LivekitServer) Stop(force bool) {
	// wait for all participants to exit
	// 等待所有参与者退出
	s.router.Drain()
	partTicker := time.NewTicker(5 * time.Second)
	waitingForParticipants := !force && s.roomManager.HasParticipants()
	for waitingForParticipants {
		<-partTicker.C
		logger.Infow("waiting for participants to exit")
		waitingForParticipants = s.roomManager.HasParticipants()
	}
	partTicker.Stop()

	if !s.running.Swap(false) {
		return
	}

	s.router.Stop()
	close(s.doneChan)

	// wait for fully closed
	<-s.closedChan
}

func (s *LivekitServer) RoomManager() *RoomManager {
	return s.roomManager
}

func (s *LivekitServer) debugGoroutines(w http.ResponseWriter, _ *http.Request) {
	_ = pprof.Lookup("goroutine").WriteTo(w, 2)
}

func (s *LivekitServer) debugInfo(w http.ResponseWriter, _ *http.Request) {
	s.roomManager.lock.RLock()
	info := make([]map[string]interface{}, 0, len(s.roomManager.rooms))
	for _, room := range s.roomManager.rooms {
		info = append(info, room.DebugInfo())
	}
	s.roomManager.lock.RUnlock()

	b, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(err.Error()))
	} else {
		_, _ = w.Write(b)
	}
}

func (s *LivekitServer) defaultHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.healthCheck(w, r)
	} else {
		http.NotFound(w, r)
	}
}

// healthCheck 健康检查
// changed 2025-03-10 cqlmq 添加未初始化节点判断
func (s *LivekitServer) healthCheck(w http.ResponseWriter, _ *http.Request) {
	stats := s.Node().Stats
	if stats == nil {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte("Node not initialized"))
		return
	}
	var updatedAt = time.Unix(stats.UpdatedAt, 0)
	if time.Since(updatedAt) > 4*time.Second {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(fmt.Sprintf("Not Ready\nNode Updated At %s", updatedAt)))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// worker to perform periodic tasks per node
func (s *LivekitServer) backgroundWorker() {
	roomTicker := time.NewTicker(1 * time.Second)
	defer roomTicker.Stop()
	for {
		select {
		case <-s.doneChan:
			return
		case <-roomTicker.C:
			s.roomManager.CloseIdleRooms()
		}
	}
}

// configureMiddlewares 配置中间件
// 将所有中间件添加到Negroni实例中
func configureMiddlewares(handler http.Handler, middlewares ...negroni.Handler) *negroni.Negroni {
	n := negroni.New()
	for _, m := range middlewares {
		n.Use(m)
	}
	n.UseHandler(handler)
	return n
}
