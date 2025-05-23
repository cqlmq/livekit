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

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/telemetry/prometheus"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/rpc"
	"github.com/livekit/psrpc"
	"github.com/livekit/psrpc/pkg/metadata"
	"github.com/livekit/psrpc/pkg/middleware"
)

// SessionHandler 定义了会话处理接口
// counterfeiter:generate . SessionHandler
type SessionHandler interface {
	Logger(ctx context.Context) logger.Logger
	HandleSession(
		ctx context.Context,
		pi routing.ParticipantInit,
		connectionID livekit.ConnectionID,
		requestSource routing.MessageSource,
		responseSink routing.MessageSink,
	) error
}

// SignalServer 定义了信令服务器
type SignalServer struct {
	server rpc.TypedSignalServer // 信令服务器
	nodeID livekit.NodeID        // 节点ID
}

// NewSignalServer 创建一个信令服务器
func NewSignalServer(
	nodeID livekit.NodeID,
	region string,
	bus psrpc.MessageBus,
	config config.SignalRelayConfig,
	sessionHandler SessionHandler,
) (*SignalServer, error) {
	s, err := rpc.NewTypedSignalServer(
		nodeID,
		&signalService{region, sessionHandler, config},
		bus,
		middleware.WithServerMetrics(rpc.PSRPCMetricsObserver{}),
		psrpc.WithServerChannelSize(config.StreamBufferSize),
	)
	if err != nil {
		return nil, err
	}
	return &SignalServer{s, nodeID}, nil
}

// NewDefaultSignalServer 创建一个默认的信号服务器
func NewDefaultSignalServer(
	currentNode routing.LocalNode, // 当前节点
	bus psrpc.MessageBus, // 消息总线
	config config.SignalRelayConfig, // 信号服务器配置
	router routing.Router, // 路由器
	roomManager *RoomManager, // 房间管理器
) (r *SignalServer, err error) {
	return NewSignalServer(currentNode.NodeID(), currentNode.Region(), bus, config, &defaultSessionHandler{currentNode, router, roomManager})
}

// defaultSessionHandler 默认会话处理器
type defaultSessionHandler struct {
	currentNode routing.LocalNode
	router      routing.Router
	roomManager *RoomManager
}

// Logger 返回日志记录器
func (s *defaultSessionHandler) Logger(ctx context.Context) logger.Logger {
	return logger.GetLogger()
}

// HandleSession 处理会话
// 这里真正完成信令处理
func (s *defaultSessionHandler) HandleSession(
	ctx context.Context,
	pi routing.ParticipantInit,
	connectionID livekit.ConnectionID,
	requestSource routing.MessageSource,
	responseSink routing.MessageSink,
) error {
	// 1. 增加参与者计数
	prometheus.IncrementParticipantRtcInit(1)

	// 2. 获取RTC节点
	rtcNode, err := s.router.GetNodeForRoom(ctx, livekit.RoomName(pi.CreateRoom.Name))
	if err != nil {
		return err
	}

	// 3. 检查RTC节点是否正确，如果不是当前节点的消息，则返回错误
	if livekit.NodeID(rtcNode.Id) != s.currentNode.NodeID() {
		err = routing.ErrIncorrectRTCNode
		logger.Errorw("called participant on incorrect node", err,
			"rtcNode", rtcNode,
		)
		return err
	}

	// 4. 开始会话
	// 收发消息通道&参与者信息 -> 房间管理器的StartSession方法
	return s.roomManager.StartSession(ctx, pi, requestSource, responseSink, false)
}

// Start 启动信号服务器
// 注册所有节点主题
func (s *SignalServer) Start() error {
	logger.Debugw("starting relay signal server", "topic", s.nodeID)
	return s.server.RegisterAllNodeTopics(s.nodeID)
}

func (r *SignalServer) Stop() {
	r.server.Kill()
}

type signalService struct {
	region         string
	sessionHandler SessionHandler
	config         config.SignalRelayConfig
}

// RelaySignal 处理信令请求
func (r *signalService) RelaySignal(stream psrpc.ServerStream[*rpc.RelaySignalResponse, *rpc.RelaySignalRequest]) (err error) {
	// 1. 获取初始化请求
	// 2. 验证会话信息
	// 3. 建立消息通道
	// 4. 处理会话
	req, ok := <-stream.Channel()
	if !ok {
		return nil
	}

	ss := req.StartSession
	if ss == nil {
		return errors.New("expected start session message")
	}

	// 5. 从会话信息中读取参与者初始化信息
	pi, err := routing.ParticipantInitFromStartSession(ss, r.region)
	if err != nil {
		return errors.Wrap(err, "failed to read participant from session")
	}

	// 6. 创建日志记录器
	l := r.sessionHandler.Logger(stream.Context()).WithValues(
		"room", ss.RoomName,
		"participant", ss.Identity,
		"connID", ss.ConnectionId,
	)

	stream.Hijack() // 劫持流 以后好好研究一下，调用后，stream在处理时，不会调用Close方法?

	// 7. 创建信令消息写入器
	sink := routing.NewSignalMessageSink(routing.SignalSinkParams[*rpc.RelaySignalResponse, *rpc.RelaySignalRequest]{
		Logger:       l,
		Stream:       stream,
		Config:       r.config,
		Writer:       signalResponseMessageWriter{},
		ConnectionID: livekit.ConnectionID(ss.ConnectionId),
	})

	// 8. 创建信令消息通道
	reqChan := routing.NewDefaultMessageChannel(livekit.ConnectionID(ss.ConnectionId))

	// 9. 启动协程，复制信令流到消息通道
	go func() {
		err := routing.CopySignalStreamToMessageChannel[*rpc.RelaySignalResponse, *rpc.RelaySignalRequest](
			stream,
			reqChan,
			signalRequestMessageReader{},
			r.config,
		)
		l.Debugw("signal stream closed", "error", err)

		reqChan.Close()
	}()

	// copy the context to prevent a race between the session handler closing
	// and the delivery of any parting messages from the client. take care to
	// copy the incoming rpc headers to avoid dropping any session vars.
	// 拷贝上下文，防止会话处理关闭和客户端发送的任何离开消息之间出现竞争。
	// 拷贝入站RPC头，避免丢弃任何会话变量。
	// 10. 处理会话
	ctx := metadata.NewContextWithIncomingHeader(context.Background(), metadata.IncomingHeader(stream.Context()))
	err = r.sessionHandler.HandleSession(ctx, *pi, livekit.ConnectionID(ss.ConnectionId), reqChan, sink)
	if err != nil {
		sink.Close()
		l.Errorw("could not handle new participant", err)
	}
	return
}

type signalResponseMessageWriter struct{}

func (e signalResponseMessageWriter) Write(seq uint64, close bool, msgs []proto.Message) *rpc.RelaySignalResponse {
	r := &rpc.RelaySignalResponse{
		Seq:       seq,
		Responses: make([]*livekit.SignalResponse, 0, len(msgs)),
		Close:     close,
	}
	for _, m := range msgs {
		r.Responses = append(r.Responses, m.(*livekit.SignalResponse))
	}
	return r
}

type signalRequestMessageReader struct{}

func (e signalRequestMessageReader) Read(rm *rpc.RelaySignalRequest) ([]proto.Message, error) {
	msgs := make([]proto.Message, 0, len(rm.Requests))
	for _, m := range rm.Requests {
		msgs = append(msgs, m)
	}
	return msgs, nil
}
